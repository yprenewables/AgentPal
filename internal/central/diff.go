package central

import (
	"agentpal/internal/codex"
	"agentpal/internal/types"
	"os"
	"path/filepath"
	"sort"
)

func Compare(codexDir string, manifest types.CenterManifest) (types.ConfigDiff, error) {
	localSkills, err := localSkills(codexDir)
	if err != nil {
		return types.ConfigDiff{}, err
	}
	remoteSkills := map[string]types.CenterSkill{}
	for _, item := range manifest.Skills {
		remoteSkills[item.Name] = item
	}
	items := []types.DiffItem{}
	for name, remote := range remoteSkills {
		local, ok := localSkills[name]
		status := types.DiffNew
		if ok && local.SHA256 == remote.SHA256 {
			status = types.DiffSame
		} else if ok {
			status = types.DiffChanged
		}
		items = append(items, types.DiffItem{Kind: "skill", Name: name, Status: status, Selected: status == types.DiffNew || status == types.DiffChanged})
	}
	for name := range localSkills {
		if _, ok := remoteSkills[name]; !ok {
			items = append(items, types.DiffItem{Kind: "skill", Name: name, Status: types.DiffLocalOnly})
		}
	}
	localMCP, err := codex.ReadMCP(filepath.Join(codexDir, "config.toml"))
	if err != nil {
		return types.ConfigDiff{}, err
	}
	localMap := map[string]string{}
	for _, item := range localMCP {
		localMap[item.Name] = item.SHA256
	}
	remoteMap := map[string]types.CenterMCP{}
	for _, item := range manifest.MCP {
		remoteMap[item.Name] = item
	}
	for name, remote := range remoteMap {
		local, ok := localMap[name]
		status := types.DiffNew
		if ok && local == remote.SHA256 {
			status = types.DiffSame
		} else if ok {
			status = types.DiffChanged
		}
		items = append(items, types.DiffItem{Kind: "mcp", Name: name, Status: status, Selected: status == types.DiffNew || status == types.DiffChanged})
	}
	for name := range localMap {
		if _, ok := remoteMap[name]; !ok {
			items = append(items, types.DiffItem{Kind: "mcp", Name: name, Status: types.DiffLocalOnly})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind == items[j].Kind {
			return items[i].Name < items[j].Name
		}
		return items[i].Kind < items[j].Kind
	})
	return types.ConfigDiff{Items: items}, nil
}

type localSkill struct{ SHA256 string }

func localSkills(dir string) (map[string]localSkill, error) {
	result := map[string]localSkill{}
	root := filepath.Join(dir, "skills")
	if info, err := os.Stat(root); os.IsNotExist(err) || !info.IsDir() {
		return result, nil
	} else if err != nil {
		return nil, err
	}
	entries, _ := scanSkills(root, nil)
	for _, entry := range entries {
		result[entry.Name] = localSkill{SHA256: entry.SHA256}
	}
	return result, nil
}
