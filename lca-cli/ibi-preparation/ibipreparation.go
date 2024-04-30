package ibi_preparation

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"

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
	log                 *logrus.Logger
	ops                 ops.Ops
	authFile            string
	seedImage           string
	rpmostreeClient     rpmostreeclient.IClient
	ostreeClient        ostreeclient.IClient
	seedExpectedVersion string
	pullSecretFile      string
	precacheBestEffort  bool
	precacheDisabled    bool
	skipShutdown        bool
}

func NewIBIPrepare(log *logrus.Logger, ops ops.Ops, rpmostreeClient rpmostreeclient.IClient,
	ostreeClient ostreeclient.IClient, seedImage, authFile, pullSecretFile, seedExpectedVersion string,
	precacheBestEffort, precacheDisabled, skipShutdown bool) *IBIPrepare {
	return &IBIPrepare{
		log:                 log,
		ops:                 ops,
		authFile:            authFile,
		pullSecretFile:      pullSecretFile,
		seedImage:           seedImage,
		rpmostreeClient:     rpmostreeClient,
		ostreeClient:        ostreeClient,
		seedExpectedVersion: seedExpectedVersion,
		precacheDisabled:    precacheDisabled,
		precacheBestEffort:  precacheBestEffort,
		skipShutdown:        skipShutdown,
	}
}

func (i *IBIPrepare) Run() error {
	// Pull seed image
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
