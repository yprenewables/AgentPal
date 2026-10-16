package modelcatalog

import (
	"regexp"
	"sort"
	"strings"
)

// Info 描述一个模型在 Codex 模型目录中需要的可选能力参数。
type Info struct {
	Slug             string
	Provider         string
	ContextWindow    int64
	InputModalities  []string
	ReasoningEfforts []string
	DefaultEffort    string
}

// Entry 是模型目录中的一条记录。
type Entry struct {
	ID        string
	Canonical string
	Provider  string
	Info      Info
}

// Catalog 保存可查询的模型条目。
type Catalog struct{ entries []Entry }

func NewCatalog(entries []Entry) *Catalog { return &Catalog{entries: entries} }

var (
	variantSuffix = regexp.MustCompile(`:(batch|free|extended|thinking|online|nitro)$`)
	versionPair   = regexp.MustCompile(`-(\d+)-(\d+)`)
)

// normalized 生成用于比对的名字候选，双向比较前先在两侧展开。
func normalized(name string) []string {
	s := strings.ToLower(strings.TrimSpace(name))
	base := strings.TrimPrefix(s, "~")
	base = variantSuffix.ReplaceAllString(base, "")
	base = strings.TrimSuffix(base, "-latest")
	set := map[string]bool{}
	for _, v := range []string{s, base} {
		set[v] = true
		set[versionPair.ReplaceAllString(v, "-$1.$2")] = true
		set[strings.ReplaceAll(v, ":", "-")] = true
	}
	for v := range set {
		set[versionPair.ReplaceAllString(v, "-$1.$2")] = true
	}
	out := make([]string, 0, len(set))
	for v := range set {
		if v != "" {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func intersects(a, b []string) bool {
	seen := map[string]bool{}
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if seen[v] {
			return true
		}
	}
	return false
}

func lastSegment(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

// Lookup 按 slug 查找唯一命中的条目；命中多个不同条目时返回 false，避免误匹配。
func (c *Catalog) Lookup(slug string) (Info, bool) {
	lower := strings.ToLower(strings.TrimSpace(slug))
	variants := normalized(slug)
	best, tie := 0, false
	var winner *Entry
	for i := range c.entries {
		e := &c.entries[i]
		score := 0
		for _, cand := range []string{lastSegment(e.ID), lastSegment(e.Canonical)} {
			if cand == "" {
				continue
			}
			if strings.ToLower(cand) == lower {
				score = 3
			} else if score < 2 && intersects(variants, normalized(cand)) {
				score = 2
			}
		}
		if score == 0 {
			continue
		}
		if strings.Contains(lastSegment(e.ID), ":") {
			score--
		}
		if strings.HasPrefix(e.ID, "~") {
			score--
		}
		switch {
		case score > best:
			best, winner, tie = score, e, false
		case score == best && winner != nil && winner.ID != e.ID:
			tie = true
		}
	}
	if winner == nil || tie {
		return Info{}, false
	}
	return winner.Info, true
}

// ContextWindowOverride 是 openai / anthropic 模型统一使用的上下文长度。
const ContextWindowOverride int64 = 272000

var codexEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra", "persistent"}

// CodexEfforts 返回 Codex 支持的推理档位白名单副本。
func CodexEfforts() []string { return append([]string{}, codexEfforts...) }

func isCodexEffort(value string) bool {
	for _, e := range codexEfforts {
		if e == value {
			return true
		}
	}
	return false
}

func filterModalities(in []string) []string {
	out := []string{}
	for _, m := range []string{"text", "image", "audio"} {
		for _, v := range in {
			if strings.EqualFold(v, m) {
				out = append(out, m)
				break
			}
		}
	}
	if len(out) == 0 {
		return []string{"text"}
	}
	return out
}

func filterEfforts(in []string) []string {
	out := []string{}
	for _, e := range in {
		if isCodexEffort(strings.ToLower(e)) {
			out = append(out, strings.ToLower(e))
		}
	}
	return out
}

func overridesContext(provider string) bool {
	return provider == "openai" || provider == "anthropic"
}

// BuildInfo 把接口原始字段映射为 Codex 需要的参数，并应用上下文长度覆盖规则。
func BuildInfo(id, canonical string, contextLength int64, modalities, efforts []string, defaultEffort string) (Entry, bool) {
	if strings.TrimSpace(id) == "" {
		return Entry{}, false
	}
	provider := strings.ToLower(strings.SplitN(id, "/", 2)[0])
	slug := lastSegment(id)
	if canonical != "" {
		slug = lastSegment(canonical)
	}
	window := contextLength
	if overridesContext(provider) {
		window = ContextWindowOverride
	}
	info := Info{
		Slug:             slug,
		Provider:         provider,
		ContextWindow:    window,
		InputModalities:  filterModalities(modalities),
		ReasoningEfforts: filterEfforts(efforts),
	}
	if isCodexEffort(strings.ToLower(defaultEffort)) {
		info.DefaultEffort = strings.ToLower(defaultEffort)
	}
	return Entry{ID: id, Canonical: canonical, Provider: provider, Info: info}, true
}
