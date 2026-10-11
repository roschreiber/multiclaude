package devcontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Container struct {
	ID         string
	Name       string
	HostFolder string
	WorkFolder string
	User       string
}

type dockerMount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

type dockerConfig struct {
	Labels map[string]string `json:"Labels"`
}

type dockerInfo struct {
	ID     string        `json:"Id"`
	Name   string        `json:"Name"`
	Config dockerConfig  `json:"Config"`
	Mounts []dockerMount `json:"Mounts"`
}

type metadataEntry struct {
	RemoteUser    string `json:"remoteUser"`
	ContainerUser string `json:"containerUser"`
}

func RunDocker(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", args...).Output()
}

func findContainers() ([]Container, error) {
	out, err := RunDocker("ps", "-q", "--filter", "label=devcontainer.local_folder")
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil, nil
	}

	inspectArgs := []string{"inspect"}
	inspectArgs = append(inspectArgs, ids...)
	out, err = RunDocker(inspectArgs...)
	if err != nil {
		return nil, err
	}
	var infos []dockerInfo
	err = json.Unmarshal(out, &infos)
	if err != nil {
		return nil, err
	}

	var containers []Container
	for _, info := range infos {
		hostFolder := info.Config.Labels["devcontainer.local_folder"]
		container := Container{
			ID:         info.ID,
			Name:       strings.TrimPrefix(info.Name, "/"),
			HostFolder: hostFolder,
			WorkFolder: findWorkFolder(info, hostFolder),
			User:       findUser(info.Config.Labels["devcontainer.metadata"]),
		}
		containers = append(containers, container)
	}
	return containers, nil
}

func lastPart(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimRight(path, "/")
	return path[strings.LastIndex(path, "/")+1:]
}

func findWorkFolder(info dockerInfo, hostFolder string) string {
	name := lastPart(hostFolder)
	for _, mount := range info.Mounts {
		if mount.Type == "bind" && strings.EqualFold(lastPart(mount.Source), name) {
			return mount.Destination
		}
	}
	return "/workspaces/" + name
}

func findUser(metadata string) string {
	var entries []metadataEntry
	err := json.Unmarshal([]byte(metadata), &entries)
	if err != nil {
		return ""
	}

	remoteUser := ""
	containerUser := ""
	for _, entry := range entries {
		if entry.RemoteUser != "" {
			remoteUser = entry.RemoteUser
		}
		if entry.ContainerUser != "" {
			containerUser = entry.ContainerUser
		}
	}

	if remoteUser != "" {
		return remoteUser
	}
	return containerUser
}

func hasConfig(folder string) bool {
	_, err := os.Stat(filepath.Join(folder, ".devcontainer", "devcontainer.json"))
	if err == nil {
		return true
	}
	_, err = os.Stat(filepath.Join(folder, ".devcontainer.json"))
	if err == nil {
		return true
	}
	others, _ := filepath.Glob(filepath.Join(folder, ".devcontainer", "*", "devcontainer.json"))
	return len(others) > 0
}

func cleanForCompare(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimRight(path, "/")
	return strings.ToLower(path)
}

func isInside(parent, child string) bool {
	return child == parent || strings.HasPrefix(child, parent+"/")
}

func containerForHostFolder(containers []Container, folder string) *Container {
	for i := range containers {
		if cleanForCompare(containers[i].HostFolder) == cleanForCompare(folder) {
			return &containers[i]
		}
	}
	return nil
}

func containerForContainerFolder(containers []Container, folder string) *Container {
	for i := range containers {
		if isInside(containers[i].WorkFolder, folder) {
			return &containers[i]
		}
	}
	return nil
}

func ResolveFolder(folder string, resume bool, enabled bool) (*Container, string, error) {
	info, err := os.Stat(folder)
	onHost := err == nil && info.IsDir()

	if !enabled {
		if onHost {
			return nil, folder, nil
		}
		return nil, "", fmt.Errorf("not a folder: %s", folder)
	}

	if onHost && (resume || !hasConfig(folder)) {
		return nil, folder, nil
	}

	containers, err := findContainers()
	if err != nil {
		return nil, "", fmt.Errorf("%s needs a devcontainer, but docker is not available", folder)
	}

	if onHost {
		container := containerForHostFolder(containers, folder)
		if container == nil {
			return nil, "", fmt.Errorf("the devcontainer of %s is not running. Start it first, or turn devcontainers off in settings", folder)
		}
		return container, container.WorkFolder, nil
	}

	container := containerForContainerFolder(containers, folder)
	if container == nil {
		return nil, "", fmt.Errorf("not a folder here, and no running devcontainer has it: %s", folder)
	}
	return container, folder, nil
}
