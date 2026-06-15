package update

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agentpal/internal/constants"
	"agentpal/internal/types"
)

type releaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

func Check() (types.UpdateInfo, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	url := "https://api.github.com/repos/" + constants.RepoOwner + "/" + constants.RepoName + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return types.UpdateInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", constants.AppName+"/"+constants.AppVersion)

	resp, err := client.Do(req)
	if err != nil {
		return types.UpdateInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return types.UpdateInfo{}, errors.New("GitHub release check failed: " + resp.Status)
	}

	var release releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return types.UpdateInfo{}, err
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	info := types.UpdateInfo{
		CurrentVersion: constants.AppVersion,
		LatestVersion:  latest,
		HasUpdate:      compareVersion(latest, constants.AppVersion) > 0,
		ReleaseURL:     release.HTMLURL,
	}
	for _, asset := range release.Assets {
		info.Assets = append(info.Assets, types.UpdateAsset{Name: asset.Name, URL: asset.BrowserDownloadURL, Size: asset.Size})
	}
	return info, nil
}

func compareVersion(a, b string) int {
	ap := versionParts(a)
	bp := versionParts(b)
	for i := 0; i < 3; i++ {
		if ap[i] > bp[i] {
			return 1
		}
		if ap[i] < bp[i] {
			return -1
		}
	}
	return 0
}

func versionParts(value string) [3]int {
	fields := strings.Split(strings.TrimSpace(value), ".")
	var parts [3]int
	for i := 0; i < len(fields) && i < len(parts); i++ {
		part, _ := strconv.Atoi(fields[i])
		parts[i] = part
	}
	return parts
}
