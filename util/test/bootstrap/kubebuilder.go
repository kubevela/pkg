/*
Copyright 2022 The KubeVela Authors.

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

package bootstrap

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kubevela/pkg/util/singleton"
)

// InitKubeBuilderForTest call this function to init kubebuilder for test
func InitKubeBuilderForTest(options ...InitKubeConfigOption) *rest.Config {
	var testEnv *envtest.Environment
	var cfg rest.Config

	initCfg := &initKubeBuilderConfig{}
	for _, op := range options {
		op.ApplyToConfig(initCfg)
	}

	BeforeSuite(func() {
		logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
		By("Bootstrapping Test Environment")

		testEnv = &envtest.Environment{
			ControlPlaneStartTimeout: time.Minute,
			ControlPlaneStopTimeout:  time.Minute,
			Scheme:                   scheme.Scheme,
			UseExistingCluster:       ptr.To(false),
		}
		workDir, err := os.Getwd()
		Ω(err).To(Succeed())
		if initCfg.crdPath != nil {
			path := *initCfg.crdPath
			if !filepath.IsAbs(path) {
				path = filepath.Join(workDir, *initCfg.crdPath)
			}
			testEnv.CRDDirectoryPaths = []string{path}
		}
		_cfg, err := testEnv.Start()
		cfg = *_cfg
		Ω(err).To(Succeed())
		if initCfg.onConfigLoaded != nil {
			initCfg.onConfigLoaded(_cfg)
		}
		singleton.KubeConfig.Set(&cfg)
		singleton.ReloadClients()
	})

	AfterSuite(func() {
		By("Tearing Down the Test Environment")
		Ω(testEnv.Stop()).To(Succeed())
	})

	return &cfg
}

// initKubeBuilderConfig the config for init kubebuilder for test
type initKubeBuilderConfig struct {
	crdPath        *string
	onConfigLoaded func(*rest.Config)
}

// InitKubeConfigOption the option for init kubebuilder for test
type InitKubeConfigOption interface {
	ApplyToConfig(*initKubeBuilderConfig)
}

// WithCRDPath configure the path to load CRD files
// The path is relative to the working directory
type WithCRDPath string

// ApplyToConfig apply to initKubeBuilderConfig
func (op WithCRDPath) ApplyToConfig(cfg *initKubeBuilderConfig) {
	cfg.crdPath = ptr.To(string(op))
}

// WithOnConfigLoaded configure the callback when rest.Config is bootstrapped
type WithOnConfigLoaded func(*rest.Config)

// ApplyToConfig apply to initKubeBuilderConfig
func (op WithOnConfigLoaded) ApplyToConfig(cfg *initKubeBuilderConfig) {
	cfg.onConfigLoaded = op
}
