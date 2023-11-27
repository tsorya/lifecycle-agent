package postpivot

import (
	"fmt"
	"github.com/openshift-kni/lifecycle-agent/ibu-imager/clusterinfo"
	"github.com/openshift-kni/lifecycle-agent/ibu-imager/ops"
	"github.com/openshift-kni/lifecycle-agent/internal/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/kubernetes/scheme"
	"os"
	"path/filepath"
	"testing"
)

func TestRecert(t *testing.T) {
	var (
		seedManifestData = clusterinfo.ClusterInfo{Domain: "seed.com", ClusterName: "seed", MasterIP: "192.168.127.10"}
		clusterInfoData  = clusterinfo.ClusterInfo{Domain: "sno.com", ClusterName: "sno", MasterIP: "192.168.122.10"}
		testscheme       = scheme.Scheme
		log              = logrus.New()
		mockOps          *ops.MockOps
		authFile         = "test.auth"
	)

	testcases := []struct {
		name          string
		expectedError bool
		validateFunc  func(t *testing.T)
	}{
		{
			name:          "run recert happy flow",
			expectedError: false,
			validateFunc: func(t *testing.T) {
			},
		},
	}

	for _, tc := range testcases {
		tmpDir := t.TempDir()
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Join(tmpDir, common.CertsDir), 0o700); err != nil {
				t.Errorf("failed to create certs dir, error: %v", err)
			}
			_, err := os.Create(filepath.Join(tmpDir, common.CertsDir, "ingresskey-test"))
			if err != nil {
				t.Errorf("failed to create ingress file, error: %v", err)
			}

			ctrl := gomock.NewController(t)
			mockOps = ops.NewMockOps(ctrl)
			pp := NewPostPivot(testscheme, log, mockOps, common.DefaultRecertImage, authFile,
				tmpDir, "kubeconfig")

			mockOps.EXPECT().RunUnauthenticatedEtcdServer(authFile, "recert_etcd").Times(1)
			//mockOps.EXPECT().RunInHostNamespace(gomock.Any()).Times(1)

			err = pp.recert(&clusterInfoData, &seedManifestData)
			fmt.Println("AAAAA", err)
			assert.Equal(t, err != nil, tc.expectedError)
		})
	}
}
