package ibi_preparation

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/openshift-kni/lifecycle-agent/internal/precache"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/openshift-kni/lifecycle-agent/ibu-imager/ops"
	rpmostreeclient "github.com/openshift-kni/lifecycle-agent/ibu-imager/ostreeclient"
	"github.com/openshift-kni/lifecycle-agent/internal/common"
	"github.com/openshift-kni/lifecycle-agent/internal/ostreeclient"
	"github.com/openshift-kni/lifecycle-agent/internal/precache/workload"
	"github.com/openshift-kni/lifecycle-agent/internal/prep"
)

type IBIPrepare struct {
	log                 *logrus.Logger
	ops                 ops.Ops
	authFile            string
	seedImage           string
	rpmostreeClient     rpmostreeclient.IClient
	ostreeClient        ostreeclient.IClient
	seedExpectedVersion string
	pullSecretFile      string
}

func NewIBIPrepare(log *logrus.Logger, ops ops.Ops, rpmostreeClient rpmostreeclient.IClient,
	ostreeClient ostreeclient.IClient, seedImage, authFile, pullSecretFile, seedExpectedVersion string) *IBIPrepare {
	return &IBIPrepare{
		log:                 log,
		ops:                 ops,
		authFile:            authFile,
		pullSecretFile:      pullSecretFile,
		seedImage:           seedImage,
		rpmostreeClient:     rpmostreeClient,
		ostreeClient:        ostreeClient,
		seedExpectedVersion: seedExpectedVersion,
	}
}

func (i *IBIPrepare) Run() error {
	var imageList []string
	imageListFile := "var/tmp/imageListFile"

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
		return err
	}

	// TODO: add support for mirror registry
	imageList, err := prep.ReadPrecachingList(imageListFile, "", "", false)
	if err != nil {
		err = fmt.Errorf("failed to read pre-caching image file: %s, %w", common.PathOutsideChroot(imageListFile), err)
		return err
	}

	// Change root directory to /host
	if err := syscall.Chroot(common.Host); err != nil {
		return fmt.Errorf("failed to chroot to %s, err: %w", common.Host, err)
	}

	if err := os.MkdirAll(filepath.Dir(precache.StatusFile), 0o700); err != nil {
		return fmt.Errorf("failed to create status file dir, err %w", err)
	}
	i.log.Infof("chroot %s successful", common.Host)

	status, err := workload.PullImages(imageList, i.pullSecretFile)
	if err != nil {
		return err
	}
	if err := workload.ValidatePrecache(status, false); err != nil {
		return err
	}
	return nil
}
