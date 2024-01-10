package prep

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"

	"github.com/openshift-kni/lifecycle-agent/ibu-imager/clusterinfo"
	"github.com/openshift-kni/lifecycle-agent/ibu-imager/ops"
	rpmostreeclient "github.com/openshift-kni/lifecycle-agent/ibu-imager/ostreeclient"
	"github.com/openshift-kni/lifecycle-agent/internal/common"
	"github.com/openshift-kni/lifecycle-agent/internal/ostreeclient"
	"github.com/openshift-kni/lifecycle-agent/utils"
)

// need this for unit tests
var osReadFile = os.ReadFile

// GetBootedStaterootIDFromRPMOstreeJson reads rpm-ostree.json file from the seed image
// and returns the deployment.ID of the booted stateroot
func GetBootedStaterootIDFromRPMOstreeJson(path string) (string, error) {
	data, err := osReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed reading %s: %w", path, err)
	}
	var status rpmostreeclient.Status
	if err := json.Unmarshal(data, &status); err != nil {
		return "", fmt.Errorf("failed unmarshalling %s: %w", path, err)
	}
	for _, deploy := range status.Deployments {
		if deploy.Booted {
			return deploy.ID, nil
		}
	}
	return "", fmt.Errorf("failed finding booted stateroot")
}

// GetVersionFromClusterInfoFile reads ClusterInfo file and returns the ocp version
func GetVersionFromClusterInfoFile(path string) (string, error) {
	ci := &clusterinfo.ClusterInfo{}
	if err := utils.ReadYamlOrJSONFile(path, ci); err != nil {
		return "", fmt.Errorf("failed to read and decode ClusterInfo file: %w", err)
	}
	return ci.Version, nil
}

// BuildKernelArguementsFromMCOFile reads the kernel arguments from MCO file
// and builds the string arguments that ostree admin deploy requires
func BuildKernelArgumentsFromMCOFile(path string) ([]string, error) {
	mc := &mcfgv1.MachineConfig{}
	if err := utils.ReadYamlOrJSONFile(path, mc); err != nil {
		return nil, fmt.Errorf("failed to read and decode machine config json file: %w", err)
	}

	args := make([]string, len(mc.Spec.KernelArguments)*2)
	for i, karg := range mc.Spec.KernelArguments {
		// if we don't marshal the karg, `"` won't appear in the kernel arguments after reboot
		if val, err := json.Marshal(karg); err != nil {
			return nil, fmt.Errorf("failed to marshal karg %s: %w", karg, err)
		} else {
			args[2*i] = "--karg-append"
			args[2*i+1] = string(val)
		}
	}
	return args, nil
}

// GetDeploymentDirPath return the path to ostree deploy directory e.g:
// /ostree/deploy/<osname>/deploy/<deployment.id>
func GetDeploymentDirPath(osname, deployment string) string {
	return filepath.Join(common.GetStaterootPath(osname), fmt.Sprintf("deploy/%s", deployment))
}

// GetDeploymentOriginPath return the path to .orign file e.g:
// /ostree/deploy/<osname>/deploy/<deployment.id>.origin
func GetDeploymentOriginPath(osname, deployment string) string {
	originName := fmt.Sprintf("%s.origin", deployment)
	return filepath.Join(common.GetStaterootPath(osname), fmt.Sprintf("deploy/%s", originName))
}

// RemoveETCDeletions remove the files that are listed in etc.deletions
func RemoveETCDeletions(mountpoint, osname, deployment string) error {
	file, err := os.Open(filepath.Join(common.PathOutsideChroot(mountpoint), "etc.deletions"))
	if err != nil {
		return fmt.Errorf("failed to open etc.deletions: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fileToRemove := strings.Trim(scanner.Text(), " ")
		filePath := common.PathOutsideChroot(filepath.Join(GetDeploymentDirPath(osname, deployment), fileToRemove))
		err = os.Remove(filePath)
		if err != nil {
			return fmt.Errorf("failed to remove %s: %w", filePath, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error while reading %s: %w", file.Name(), err)
	}
	return nil
}

// split the deploymentID by '-' and return the last item
// there should be at least one '-' in the deploymentID
func GetDeploymentFromDeploymentID(deploymentID string) (string, error) {
	splitted := strings.Split(deploymentID, "-")
	if len(splitted) < 2 {
		return "", fmt.Errorf(
			"failed to get deployment from deploymentID, there should be a '-' in deploymentID %s",
			deploymentID)
	}
	return splitted[len(splitted)-1], nil
}

func SetupStateroot(log logr.Logger, ops ops.Ops, ostreeClient ostreeclient.IClient,
	rpmOstreeClient rpmostreeclient.IClient, seedImage, expectedVersion, imageListFile string, ibi bool) error {
	log.Info("Start setupstateroot")

	defer ops.UnmountAndRemoveImage(seedImage)

	workspaceOutsideChroot, err := os.MkdirTemp(common.PathOutsideChroot("/var/tmp"), "")
	if err != nil {
		return fmt.Errorf("failed to create temp directory %w", err)
	}

	defer func() {
		if err := os.RemoveAll(workspaceOutsideChroot); err != nil {
			log.Error(err, "failed to cleanup workspace")
		}
	}()

	workspace, err := filepath.Rel(common.Host, workspaceOutsideChroot)
	if err != nil {
		return fmt.Errorf("failed to get workspace relative path %w", err)
	}
	log.Info("workspace:" + workspace)

	if !ibi {
		if err = ops.RemountSysroot(); err != nil {
			return fmt.Errorf("failed to remount /sysroot: %w", err)
		}

	}

	mountpoint, err := ops.RunInHostNamespace("podman", "image", "mount", seedImage)
	if err != nil {
		return fmt.Errorf("failed to mount seed image: %w", err)
	}

	ostreeRepo := filepath.Join(workspace, "ostree")
	if err = os.Mkdir(common.PathOutsideChroot(ostreeRepo), 0o755); err != nil {
		return fmt.Errorf("failed to create ostree repo directory: %w", err)
	}

	if err := ops.ExtractTarWithSELinux(
		fmt.Sprintf("%s/ostree.tgz", mountpoint), ostreeRepo,
	); err != nil {
		return fmt.Errorf("failed to extract ostree.tgz: %w", err)
	}

	// example:
	// seedBootedID: rhcos-ed4ab3244a76c6503a21441da650634b5abd25aba4255ca116782b2b3020519c.1
	// seedBootedDeployment: ed4ab3244a76c6503a21441da650634b5abd25aba4255ca116782b2b3020519c.1
	// seedBootedRef: ed4ab3244a76c6503a21441da650634b5abd25aba4255ca116782b2b3020519c
	seedBootedID, err := GetBootedStaterootIDFromRPMOstreeJson(filepath.Join(common.PathOutsideChroot(mountpoint), "rpm-ostree.json"))
	if err != nil {
		return fmt.Errorf("failed to get booted stateroot id: %w", err)
	}
	seedBootedDeployment, err := GetDeploymentFromDeploymentID(seedBootedID)
	if err != nil {
		return err
	}
	seedBootedRef := strings.Split(seedBootedDeployment, ".")[0]

	version, err := GetVersionFromClusterInfoFile(filepath.Join(common.PathOutsideChroot(mountpoint), common.ClusterInfoFileName))
	if err != nil {
		return fmt.Errorf("failed to get version from ClusterInfo: %w", err)
	}

	if version != expectedVersion {
		return fmt.Errorf("version specified in seed image (%s) differs from version in spec (%s)",
			version, expectedVersion)
	}

	osname := common.GetStaterootName(expectedVersion)

	if err = ostreeClient.PullLocal(ostreeRepo); err != nil {
		return fmt.Errorf("failed ostree pull-local: %w", err)
	}

	if err = ostreeClient.OSInit(osname); err != nil {
		return fmt.Errorf("failed ostree admin os-init: %w", err)
	}

	kargs, err := BuildKernelArgumentsFromMCOFile(filepath.Join(common.PathOutsideChroot(mountpoint), "mco-currentconfig.json"))
	if err != nil {
		return fmt.Errorf("failed to build kargs: %w", err)
	}

	if err = ostreeClient.Deploy(osname, seedBootedRef, kargs); err != nil {
		return fmt.Errorf("failed ostree admin deploy: %w", err)
	}

	deployment := ""
	if ibi {
		// in IBI case rpm-ostree doesn't see mounted ostrees so we need another way
		deployment, err = ostreeClient.GetDeployment(osname)
		if err != nil {
			return fmt.Errorf("failed to get deploymentID: %w", err)
		}

	} else {
		deploymentID, err := rpmOstreeClient.GetDeploymentID(osname)
		if err != nil {
			return fmt.Errorf("failed to get deploymentID: %w", err)
		}
		deployment, err = GetDeploymentFromDeploymentID(deploymentID)
		if err != nil {
			return err
		}
	}

	if err = common.CopyOutsideChroot(
		filepath.Join(mountpoint, fmt.Sprintf("ostree-%s.origin", seedBootedDeployment)),
		GetDeploymentOriginPath(osname, deployment),
	); err != nil {
		return fmt.Errorf("failed to restore origin file: %w", err)
	}

	if err = ops.ExtractTarWithSELinux(
		filepath.Join(mountpoint, "var.tgz"),
		common.GetStaterootPath(osname),
	); err != nil {
		return fmt.Errorf("failed to restore var directory: %w", err)
	}

	if err := ops.ExtractTarWithSELinux(
		filepath.Join(mountpoint, "etc.tgz"),
		GetDeploymentDirPath(osname, deployment),
	); err != nil {
		return fmt.Errorf("failed to extract seed etc: %w", err)
	}

	if err = RemoveETCDeletions(mountpoint, osname, deployment); err != nil {
		return fmt.Errorf("failed to process etc.deletions: %w", err)
	}

	if err := common.CopyOutsideChroot(filepath.Join(mountpoint, "containers.list"), imageListFile); err != nil {
		return fmt.Errorf("failed to copy image list file: %w", err)
	}

	return nil
}

func ReadPrecachingList(imageListFile, clusterRegistry, seedRegistry string, overrideSeedRegistry bool) (imageList []string, err error) {
	var content []byte
	content, err = os.ReadFile(common.PathOutsideChroot(imageListFile))
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	// Filter out empty lines
	for _, line := range lines {
		image := line
		if line == "" {
			continue
		}
		if overrideSeedRegistry {
			image, err = utils.ReplaceImageRegistry(image, clusterRegistry, seedRegistry)
			if err != nil {
				return nil, err
			}
		}
		imageList = append(imageList, image)
	}

	return imageList, nil
}
