/*
Copyright AppsCode Inc. and Contributors.

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

package shared

import (
	"context"
	"crypto/md5"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	scannerapi "kubeops.dev/scanner/apis/scanner/v1alpha1"

	cache "github.com/go-pkgz/expirable-cache/v2"
	passgen "gomodules.xyz/password-generator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/client-go/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	Cache                 cache.Cache[string, string]
	onlyOneInitImageCache = make(chan struct{})
)

func InitImageCache(size int, ttl time.Duration) {
	close(onlyOneInitImageCache) // panics when called twice
	Cache = cache.NewCache[string, string]().WithMaxKeys(size).WithTTL(ttl).WithLRU()
}

func PullSecretsHash(info kmapi.PullCredentials) string {
	h := fnv.New64a()
	meta.DeepHashObject(h, info)
	newHash := strconv.FormatUint(h.Sum64(), 10)
	return newHash
}

func SendScanRequest(ctx context.Context, kc client.Client, ref string, info kmapi.PullCredentials) error {
	obj := scannerapi.ImageScanRequest{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%x-%s", md5.Sum([]byte(ref)), passgen.GenerateForCharset(6, passgen.AlphaNum)),
		},
		Spec: scannerapi.ImageScanRequestSpec{
			Image:              ref,
			Namespace:          info.Namespace,
			PullSecrets:        info.SecretRefs,
			ServiceAccountName: info.ServiceAccountName,
		},
	}
	if err := kc.Create(ctx, &obj); err != nil {
		return err
	}

	if Cache != nil {
		Cache.Set(ref, PullSecretsHash(info), 0)
	}
	return nil
}
