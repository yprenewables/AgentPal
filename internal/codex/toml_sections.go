package codex

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"regexp"
	"sort"
	"strings"
)

var tableHeaderRE = regexp.MustCompile(`^\s*\[+\s*([^\]]+?)\s*\]+\s*(?:#.*)?$`)

type ConfigSections struct {
	RootKeys []string `json:"rootKeys"`
	Tables   []string `json:"tables"`
}

func InspectConfigSections(path string) (ConfigSections, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigSections{}, err
	}
	rootSet := map[string]bool{}
	tableSet := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	inRoot := true
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if match := tableHeaderRE.FindStringSubmatch(line); match != nil {
			inRoot = false
			tableSet[strings.TrimSpace(match[1])] = true
			continue
		}
		if !inRoot {
			continue
		}
		key, ok := rootKeyFromLine(line)
		if ok {
			rootSet[key] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return ConfigSections{}, err
	}
	return ConfigSections{RootKeys: sortedKeys(rootSet), Tables: sortedKeys(tableSet)}, nil
}

func MergeConfigSections(sourcePath, targetPath string, rootKeys, tables []string) error {
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	if len(rootKeys) == 0 && len(tables) == 0 {
		return os.WriteFile(targetPath, source, 0o600)
	}
	target, err := os.ReadFile(targetPath)
	if os.IsNotExist(err) {
		return os.WriteFile(targetPath, FilterConfigSections(source, rootKeys, tables), 0o600)
	}
	if err != nil {
		return err
	}
	merged, err := mergeConfigBytes(source, target, rootKeys, tables)
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, merged, 0o600)
}

func FilterConfigSections(source []byte, rootKeys, tables []string) []byte {
	sections := splitConfig(source)
	rootSet := stringSet(rootKeys)
	tableSet := stringSet(tables)
	var out [][]byte
	if len(rootSet) > 0 {
		root := filterRootAssignments(sections.root, rootSet)
		if len(bytes.TrimSpace(root)) > 0 {
			out = append(out, root)
		}
	}
	for _, section := range sections.tables {
		if tableSet[section.name] {
			out = append(out, section.bytes)
		}
	}
	return joinSections(out)
}

func mergeConfigBytes(source, target []byte, rootKeys, tables []string) ([]byte, error) {
	if len(rootKeys) == 0 && len(tables) == 0 {
		return source, nil
	}
	sourceSections := splitConfig(source)
	targetSections := splitConfig(target)
	rootSet := stringSet(rootKeys)
	tableSet := stringSet(tables)

	root := targetSections.root
	if len(rootSet) > 0 {
		selected := filterRootAssignments(sourceSections.root, rootSet)
		root = replaceRootAssignments(root, rootSet, selected)
	}

	var out [][]byte
	if len(bytes.TrimSpace(root)) > 0 {
		out = append(out, ensureTrailingNewline(root))
	}
	usedTables := map[string]bool{}
	for _, section := range targetSections.tables {
		if tableSet[section.name] {
			sourceSection, ok := sourceSections.byName[section.name]
			if ok {
				out = append(out, sourceSection.bytes)
			}
			usedTables[section.name] = true
			continue
		}
		out = append(out, section.bytes)
	}
	for _, section := range sourceSections.tables {
		if tableSet[section.name] && !usedTables[section.name] {
			out = append(out, section.bytes)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("selected config sections were not found")
	}
	return joinSections(out), nil
}

type parsedConfig struct {
	root   []byte
	tables []tableSection
	byName map[string]tableSection
}

type tableSection struct {
	name  string
	bytes []byte
}

func splitConfig(data []byte) parsedConfig {
	lines := bytes.SplitAfter(data, []byte("\n"))
	parsed := parsedConfig{byName: map[string]tableSection{}}
	var current *tableSection
	for _, line := range lines {
		plain := strings.TrimSuffix(string(line), "\n")
		plain = strings.TrimSuffix(plain, "\r")
		if match := tableHeaderRE.FindStringSubmatch(plain); match != nil {
			section := tableSection{name: strings.TrimSpace(match[1]), bytes: append([]byte{}, line...)}
			parsed.tables = append(parsed.tables, section)
			current = &parsed.tables[len(parsed.tables)-1]
			parsed.byName[section.name] = parsed.tables[len(parsed.tables)-1]
			continue
		}
		if current == nil {
			parsed.root = append(parsed.root, line...)
		} else {
			current.bytes = append(current.bytes, line...)
			parsed.tables[len(parsed.tables)-1] = *current
			parsed.byName[current.name] = *current
		}
	}
	return parsed
}

func filterRootAssignments(root []byte, selected map[string]bool) []byte {
	var out []byte
	lines := bytes.SplitAfter(root, []byte("\n"))
	for _, line := range lines {
		key, ok := rootKeyFromLine(string(line))
		if ok && selected[key] {
			out = append(out, line...)
		}
	}
	return out
}

func replaceRootAssignments(root []byte, selected map[string]bool, replacement []byte) []byte {
	var out []byte
	inserted := false
	lines := bytes.SplitAfter(root, []byte("\n"))
	for _, line := range lines {
		key, ok := rootKeyFromLine(string(line))
		if ok && selected[key] {
			if !inserted {
				out = append(out, ensureTrailingNewline(replacement)...)
				inserted = true
			}
			continue
		}
		out = append(out, line...)
	}
	if !inserted && len(bytes.TrimSpace(replacement)) > 0 {
		out = append(ensureTrailingNewline(out), ensureTrailingNewline(replacement)...)
	}
	return out
}

func rootKeyFromLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	idx := strings.Index(trimmed, "=")
	if idx <= 0 {
		return "", false
	}
	key := strings.TrimSpace(trimmed[:idx])
	if key == "" || strings.ContainsAny(key, " []{}") {
		return "", false
	}
	return strings.Trim(key, `"'`), true
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			set[value] = true
		}
	}
	return set
}

func joinSections(sections [][]byte) []byte {
	var out []byte
	for idx, section := range sections {
		if idx > 0 && len(out) > 0 && !bytes.HasSuffix(out, []byte("\n\n")) {
			out = append(ensureTrailingNewline(out), '\n')
		}
		out = append(out, ensureTrailingNewline(section)...)
	}
	return out
}

func ensureTrailingNewline(data []byte) []byte {
	if len(data) == 0 || bytes.HasSuffix(data, []byte("\n")) {
		return data
	}
	return append(data, '\n')
}
