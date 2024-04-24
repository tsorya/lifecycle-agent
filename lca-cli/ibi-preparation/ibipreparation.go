package ibi_preparation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"

	"github.com/openshift/assisted-installer/shared_ops"

	"github.com/openshift-kni/lifecycle-agent/internal/common"
	"github.com/openshift-kni/lifecycle-agent/internal/ostreeclient"
	"github.com/openshift-kni/lifecycle-agent/internal/precache"
	"github.com/openshift-kni/lifecycle-agent/internal/precache/workload"
	"github.com/openshift-kni/lifecycle-agent/internal/prep"
	"github.com/openshift-kni/lifecycle-agent/lca-cli/ops"
	rpmostreeclient "github.com/openshift-kni/lifecycle-agent/lca-cli/ostreeclient"
)

const imageListFile = "var/tmp/imageListFile"

type IBIPrepare struct {
	log                        *logrus.Logger
	ops                        ops.Ops
	authFile                   string
	seedImage                  string
	rpmostreeClient            rpmostreeclient.IClient
	ostreeClient               ostreeclient.IClient
	hostCommandsExecutor       ops.Execute
	seedExpectedVersion        string
	pullSecretFile             string
	precacheBestEffort         bool
	precacheDisabled           bool
	skipShutdown               bool
	installationDisk           string
	shouldCreateExtraPartition bool
	extraPartitionLabel        string
	extraPartitionStart        string
	extraPartitionNumber       int
}

func NewIBIPrepare(log *logrus.Logger, ops ops.Ops, rpmostreeClient rpmostreeclient.IClient,
	ostreeClient ostreeclient.IClient, hostCommandsExecutor ops.Execute,
	seedImage, authFile, pullSecretFile, seedExpectedVersion,
	installationDisk, extraPartitionLabel, extraPartitionStart string,
	precacheBestEffort, precacheDisabled, skipShutdown, shouldCreateExtraPartition bool,
	extraPartitionNumber int) *IBIPrepare {
	return &IBIPrepare{
		log:                        log,
		ops:                        ops,
		authFile:                   authFile,
		pullSecretFile:             pullSecretFile,
		seedImage:                  seedImage,
		rpmostreeClient:            rpmostreeClient,
		ostreeClient:               ostreeClient,
		seedExpectedVersion:        seedExpectedVersion,
		precacheDisabled:           precacheDisabled,
		precacheBestEffort:         precacheBestEffort,
		skipShutdown:               skipShutdown,
		hostCommandsExecutor:       hostCommandsExecutor,
		installationDisk:           installationDisk,
		shouldCreateExtraPartition: shouldCreateExtraPartition,
		extraPartitionLabel:        extraPartitionLabel,
		extraPartitionStart:        extraPartitionStart,
		extraPartitionNumber:       extraPartitionNumber,
	}
}

func (i *IBIPrepare) Run() error {
	// Pull seed image
	if err := i.diskPreparation(); err != nil {
		return fmt.Errorf("failed to prepare disk: %w", err)
	}

	i.log.Info("Pulling seed image")
	if _, err := i.ops.RunInHostNamespace("podman", "pull", "--authfile", i.authFile, i.seedImage); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// TODO: change to logrus after refactoring the code in controllers and moving to logrus
	log := logr.Logger{}
	common.OstreeDeployPathPrefix = "/mnt/"
	// Setup state root
	if err := prep.SetupStateroot(log, i.ops, i.ostreeClient, i.rpmostreeClient,
		i.seedImage, i.seedExpectedVersion, imageListFile, true); err != nil {
		return fmt.Errorf("failed to setup stateroot: %w", err)
	}

	if err := i.precacheFlow(imageListFile); err != nil {
		return fmt.Errorf("failed to precache: %w", err)
	}

	return i.shutdown()
}

func (i *IBIPrepare) precacheFlow(imageListFile string) error {
	// TODO: add support for mirror registry
	if i.precacheDisabled {
		i.log.Info("Precache disabled, skipping it")
		return nil
	}

	i.log.Info("Precaching imaging")
	imageList, err := prep.ReadPrecachingList(imageListFile, "", "", false)
	if err != nil {
		err = fmt.Errorf("failed to read pre-caching image file: %s, %w", common.PathOutsideChroot(imageListFile), err)
		return err
	}

	// Change root directory to /host
	unchroot, err := i.ops.Chroot(common.Host)
	if err != nil {
		return fmt.Errorf("failed to chroot to %s, err: %w", common.Host, err)

	}

	i.log.Infof("chroot %s successful", common.Host)
	if err := os.MkdirAll(filepath.Dir(precache.StatusFile), 0o700); err != nil {
		return fmt.Errorf("failed to create status file dir, err %w", err)
	}

	if err := workload.Precache(imageList, i.pullSecretFile, i.precacheBestEffort); err != nil {
		return fmt.Errorf("failed to start precache: %w", err)
	}

	return unchroot()
}

func (i *IBIPrepare) shutdown() error {
	if i.skipShutdown {
		i.log.Info("Skipping shutdown")
		return nil
	}
	i.log.Info("Shutting down the host")
	if _, err := i.ops.RunInHostNamespace("shutdown", "now"); err != nil {
		return fmt.Errorf("failed to shutdown the host: %w", err)
	}
	return nil
}

func (i *IBIPrepare) createExtraPartition() error {
	if _, err := i.hostCommandsExecutor.Execute("bash", "-c",
		strings.Join([]string{"sfdisk", i.installationDisk, "<<<", "write"}, " ")); err != nil {
		return fmt.Errorf("failed to create extra partition: %w", err)
	}
	if _, err := i.hostCommandsExecutor.Execute("sgdisk", "--new",
		fmt.Sprintf("%d:%s", i.extraPartitionNumber, i.extraPartitionStart),
		"--change-name", fmt.Sprintf("%d:%s", i.extraPartitionNumber, i.extraPartitionLabel),
		i.installationDisk); err != nil {
		return fmt.Errorf("failed to create extra partition: %w", err)
	}

	extraPartitionPath, err := i.getExtraPartitionPath(i.installationDisk)
	if err != nil {
		return fmt.Errorf("failed to get extra partition path: %w", err)
	}

	if _, err := i.hostCommandsExecutor.Execute("mkfs.xfs", "-f", extraPartitionPath); err != nil {
		return fmt.Errorf("failed to create extra partition: %w", err)
	}

	return nil
}

func (i *IBIPrepare) diskCleanup() error {
	i.log.Info("Start cleaning up disk")
	cleanup := shared_ops.NewCleanupDevice(i.log, shared_ops.NewDiskOps(i.log, i.hostCommandsExecutor))
	if err := cleanup.CleanupInstallDevice(i.installationDisk); err != nil {
		return fmt.Errorf("failed to cleanup disk: %w", err)
	}

	i.log.Info("Disk was successfully cleaned up")
	return nil
}

func (i *IBIPrepare) setupContainersFolderCommands() []*ops.CMD {
	i.log.Info("Setting up containers folder")
	var cmds []*ops.CMD
	cmds = append(cmds, ops.NewCMD("chattr", "-i", "/mnt/"))
	cmds = append(cmds, ops.NewCMD("mkdir", "-p", "/mnt/containers"))
	cmds = append(cmds, ops.NewCMD("chattr", "+i", "/mnt/"))
	cmds = append(cmds, ops.NewCMD("mount", "-o", "bind", "/mnt/containers", "/var/lib/containers"))
	return cmds
}

func (i *IBIPrepare) prepareDiskEnvironment() error {
	if i.shouldCreateExtraPartition {
		if err := i.createExtraPartition(); err != nil {
			return fmt.Errorf("failed to create extra partition: %w", err)
		}
	}

	var cmds []*ops.CMD
	cmds = append(cmds, ops.NewCMD("growpart", i.installationDisk, "4"))
	cmds = append(cmds, ops.NewCMD("mount", "/dev/disk/by-partlabel/root", "/mnt"))
	cmds = append(cmds, ops.NewCMD("mount", "/dev/disk/by-partlabel/boot", "/mnt/boot"))
	cmds = append(cmds, ops.NewCMD("xfs_growfs", "/dev/disk/by-partlabel/root"))

	if i.shouldCreateExtraPartition {
		cmds = append(cmds, ops.NewCMD("mount",
			fmt.Sprintf("/dev/disk/by-partlabel/%s", i.extraPartitionLabel), "/mnt/var/lib/containers"))
	} else {
		cmds = append(cmds, i.setupContainersFolderCommands()...)
	}
	cmds = append(cmds, ops.NewCMD("restorecon", "-R", "/mnt/var/lib/containers"))

	if err := i.ops.RunListOfCommands(cmds); err != nil {
		return fmt.Errorf("failed to grow root partition: %w", err)
	}

	return nil
}

func (i *IBIPrepare) diskPreparation() error {
	i.log.Info("Start preparing disk")

	if err := i.diskCleanup(); err != nil {
		return err
	}

	if _, err := i.hostCommandsExecutor.Execute("coreos-installer", "install", i.installationDisk); err != nil {
		return fmt.Errorf("failed to write image to disk: %w", err)
	}

	if err := i.prepareDiskEnvironment(); err != nil {
		return fmt.Errorf("failed to prepare disk environment: %w", err)
	}

	i.log.Info("Disk was successfully prepared")

	return nil
}

func (i *IBIPrepare) getExtraPartitionPath(device string) (string, error) {
	type blockDevice struct {
		Name     string        `json:"name,omitempty"`
		Path     string        `json:"path,omitempty"`
		Children []blockDevice `json:"children,omitempty"`
	}
	var disks struct {
		Blockdevices []*blockDevice `json:"blockdevices"`
	}

	ret, err := i.hostCommandsExecutor.Execute("lsblk", device, "--json", "-O")
	if err != nil {
		return "", fmt.Errorf("failed to run lsblk: %w", err)
	}
	if err = json.Unmarshal([]byte(ret), &disks); err != nil {
		return "", fmt.Errorf("failed to unmarshal lsblk output: %w", err)
	}

	if len(disks.Blockdevices[0].Children) < i.extraPartitionNumber {
		return "", fmt.Errorf("not enough partitions in %s", i.installationDisk)
	}

	return disks.Blockdevices[0].Children[i.extraPartitionNumber-1].Path, nil
}
