/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/openshift-kni/lifecycle-agent/internal/common"
	"github.com/openshift-kni/lifecycle-agent/internal/ostreeclient"
	ibipreparation "github.com/openshift-kni/lifecycle-agent/lca-cli/ibi-preparation"
	"github.com/openshift-kni/lifecycle-agent/lca-cli/ops"
	ostree "github.com/openshift-kni/lifecycle-agent/lca-cli/ostreeclient"
)

// ibi represents the ibi preparation command
var ibi = &cobra.Command{
	Use:   "ibi",
	Short: "prepare ibi",
	Run: func(cmd *cobra.Command, args []string) {
		runIBI()
	},
}

var (
	seedImage            string
	seedVersion          string
	pullSecretFile       string
	precacheBestEffort   bool
	precacheDisabled     bool
	skipShutdown         bool
	extraPartitionNumber int
	extraPartitionStart  string
	extraPartitionLabel  string
	createExtraPartition bool
	installationDisk     string
)

func init() {

	// Add create command
	rootCmd.AddCommand(ibi)

	ibi.Flags().StringVarP(&seedImage, "seed-image", "s", "", "Seed image.")
	ibi.Flags().StringVarP(&seedVersion, "seed-version", "", "", "Seed version.")
	ibi.Flags().StringVarP(&authFile, "authfile", "a", "", "The path to the authentication file of the container registry of seed image.")
	ibi.Flags().StringVarP(&pullSecretFile, "pullSecretFile", "p", "", "The path to the pull secret file for precache process.")
	ibi.Flags().BoolVarP(&precacheBestEffort, "precache-best-effort", "", false, "Set image precache to best effort mode")
	ibi.Flags().BoolVarP(&precacheDisabled, "precache-disabled", "", false, "Disable precaching, no image precaching will run")
	ibi.Flags().BoolVarP(&skipShutdown, "skip-shutdown", "", false, "Skip shutdown of the host after the preparation process is done. Useful for debugging.")
	ibi.Flags().StringVarP(&installationDisk, "installation-disk", "", "", "The disk to install the image on.")
	ibi.Flags().IntVarP(&extraPartitionNumber, "extra-partition-number", "", 5, "The number of the extra partition to create.")
	ibi.Flags().StringVarP(&extraPartitionStart, "extra-partition-start", "", "40G", "The start of the extra partition to create.")
	ibi.Flags().StringVarP(&extraPartitionLabel, "extra-partition-label", "", "varlibcontainers", "The label of the extra partition to create.")
	ibi.Flags().BoolVarP(&createExtraPartition, "create-extra-partition", "", true, "Create an extra partition on the installation disk.")

	ibi.MarkFlagRequired("seed-image")
	ibi.MarkFlagRequired("seed-version")
	ibi.MarkFlagRequired("authfile")
	ibi.MarkFlagRequired("pullSecretFile")
}

func runIBI() {
	log.Info("IBI preparation process has started")
	hostCommandsExecutor := ops.NewChrootExecutor(log, true, common.Host)
	rpmOstreeClient := ostree.NewClient("lca-cli", hostCommandsExecutor)
	ostreeClient := ostreeclient.NewClient(hostCommandsExecutor, true)

	ibiRunner := ibipreparation.NewIBIPrepare(log, ops.NewOps(log, hostCommandsExecutor), rpmOstreeClient, ostreeClient,
		hostCommandsExecutor, seedImage, authFile, pullSecretFile, seedVersion,
		installationDisk, extraPartitionLabel, extraPartitionStart,
		precacheBestEffort, precacheDisabled, skipShutdown, createExtraPartition, extraPartitionNumber)
	if err := ibiRunner.Run(); err != nil {
		log.Fatal(err)
	}

	log.Info("IBI preparation process finished successfully!")
}
