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

package menu

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	rsapi "kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
)

func RenderMenu(driver *UserMenuDriver, req *rsapi.RenderMenuRequest) (*rsapi.Menu, error) {
	switch req.Mode {
	case rsapi.MenuAccordion:
		return driver.Get(req.Menu)
	case rsapi.MenuGallery:
		return GetGalleryMenu(driver, req.Menu)
	case rsapi.MenuDropDown:
		return GetDropDownMenu(driver, req)
	default:
		return nil, apierrors.NewBadRequest(fmt.Sprintf("unknown menu mode %s", req.Mode))
	}
}
