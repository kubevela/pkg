/*
Copyright 2023 The KubeVela Authors.

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

package cert_test

import (
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubevela/pkg/util/cert"
)

func TestGenerateBadSelfSignedCertificate(t *testing.T) {
	ca := &x509.Certificate{}
	_, _, err := cert.GenerateSelfSignedCertificate(-1, ca)
	require.Error(t, err)
	_, _, err = cert.GenerateSelfSignedCertificate(256, ca)
	require.Error(t, err)
}

func TestGenerateCertificateRequest(t *testing.T) {
	_, _, err := cert.GenerateCertificateRequest(-1, "", nil)
	require.Error(t, err)
	_, _, err = cert.GenerateCertificateRequest(256, "kubevela", nil)
	require.Error(t, err)
	_, _, err = cert.GenerateCertificateRequest(2048, "kubevela", nil)
	require.NoError(t, err)
}

func TestGenerateSelfSignedCertificateLocally(t *testing.T) {
	_, _, err := cert.GenerateDefaultSelfSignedCertificateLocally()
	require.NoError(t, err)
}
