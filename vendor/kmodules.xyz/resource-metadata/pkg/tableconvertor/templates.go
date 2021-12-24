/*
Copyright AppsCode Inc. and Contributors

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

package tableconvertor

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	meta_util "kmodules.xyz/client-go/meta"
	"kmodules.xyz/resource-metadata/pkg/tableconvertor/printers"
	resourcemetrics "kmodules.xyz/resource-metrics"

	"github.com/Masterminds/sprig/v3"
	prom_op "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"gomodules.xyz/encoding/json"
	"gomodules.xyz/jsonpath"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	metatable "k8s.io/apimachinery/pkg/api/meta/table"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/duration"
)

const UnknownValue = "<unknown>"

var templateFns = sprig.TxtFuncMap()

func init() {
	templateFns["jp"] = jsonpathFn
	templateFns["k8s_fmt_selector"] = formatLabelSelectorFn
	templateFns["k8s_fmt_label"] = formatLabelsFn
	templateFns["k8s_age"] = ageFn
	templateFns["k8s_svc_ports"] = servicePortsFn
	templateFns["k8s_container_ports"] = containerPortFn
	templateFns["k8s_container_images"] = containerImagesFn
	templateFns["k8s_volumes"] = volumesFn
	templateFns["k8s_volumeMounts"] = volumeMountsFn
	templateFns["k8s_duration"] = durationFn
	templateFns["fmt_list"] = fmtListFn
	templateFns["prom_ns_selector"] = promNamespaceSelectorFn
	templateFns["map_key_count"] = mapKeyCountFn
	templateFns["rbac_subjects"] = rbacSubjects
	templateFns["cert_validity"] = certificateValidity
	templateFns["k8s_fmt_resource_cpu"] = formatResourceCPUFn
	templateFns["k8s_fmt_resource_memory"] = formatResourceMemoryFn
	// ref: https://github.com/kmodules/resource-metrics/blob/bf6b257f8922a5572ccd20bf1cbab6bbedf4fcb4/template.go#L26-L36
	for name, fn := range resourcemetrics.TxtFuncMap() {
		templateFns[name] = fn
	}
}

// TxtFuncMap returns a 'text/template'.FuncMap
func TxtFuncMap() template.FuncMap {
	gfm := make(map[string]interface{}, len(templateFns))
	for k, v := range templateFns {
		gfm[k] = v
	}
	return gfm
}

func jsonpathFn(expr string, data interface{}, jsonoutput ...bool) (interface{}, error) {
	enableJSONoutput := len(jsonoutput) > 0 && jsonoutput[0]

	jp := jsonpath.New("jp")
	if err := jp.Parse(expr); err != nil {
		return nil, fmt.Errorf("unrecognized column definition %q", expr)
	}
	jp.AllowMissingKeys(true)
	jp.EnableJSONOutput(enableJSONoutput)

	var buf bytes.Buffer
	err := jp.Execute(&buf, data)
	if err != nil {
		return nil, err
	}

	if enableJSONoutput {
		var v []interface{}
		err = json.Unmarshal(buf.Bytes(), &v)
		return v, err
	}
	return buf.String(), err
}

func formatLabelSelectorFn(data interface{}) (string, error) {
	var sel metav1.LabelSelector
	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &sel)
		if err != nil {
			return "", err
		}
	} else if _, ok := data.(map[string]interface{}); ok {
		err := meta_util.DecodeObject(data, &sel)
		if err != nil {
			return "", err
		}
	}
	return metav1.FormatLabelSelector(&sel), nil
}

func formatLabelsFn(data interface{}) (string, error) {
	var lbl map[string]string
	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &lbl)
		if err != nil {
			return "", err
		}
	} else if _, ok := data.(map[string]interface{}); ok {
		err := meta_util.DecodeObject(data, &lbl)
		if err != nil {
			return "", err
		}
	}
	return labels.FormatLabels(lbl), nil
}

func ageFn(data interface{}) (string, error) {
	var timestamp metav1.Time
	if s, ok := data.(string); ok && s != "" {
		err := timestamp.UnmarshalQueryParameter(s)
		if err != nil {
			return "", err
		}
	} else if v, ok := data.(metav1.Time); ok {
		timestamp = v
	}
	return metatable.ConvertToHumanReadableDateType(timestamp), nil
}

func servicePortsFn(data interface{}) (string, error) {
	var ports []core.ServicePort

	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &ports)
		if err != nil {
			return "", err
		}
	} else if _, ok := data.([]interface{}); ok {
		// includes IntOrString, so meta_util.DecodeObject() can't be used.
		data, err := json.Marshal(data)
		if err != nil {
			return "", err
		}
		err = json.Unmarshal(data, &ports)
		if err != nil {
			return "", err
		}
	}
	return printers.MakeServicePortString(ports), nil
}

func containerPortFn(data interface{}) (string, error) {
	var ports []core.ContainerPort
	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &ports)
		if err != nil {
			return "", err
		}
	} else if _, ok := data.([]interface{}); ok {
		err := meta_util.DecodeObject(data, &ports)
		if err != nil {
			return "", err
		}
	}

	pieces := make([]string, len(ports))
	for ix := range ports {
		port := &ports[ix]
		pieces[ix] = fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol)
		if port.HostPort > 0 {
			pieces[ix] = fmt.Sprintf("%d:%d/%s", port.ContainerPort, port.HostPort, port.Protocol)
		}
	}
	return strings.Join(pieces, ","), nil
}

func volumesFn(data interface{}) (string, error) {
	var volumes []core.Volume
	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &volumes)
		if err != nil {
			return "", err
		}
	} else if _, ok := data.([]interface{}); ok {
		err := meta_util.DecodeObject(data, &volumes)
		if err != nil {
			return "", err
		}
	}

	ss := "["
	for i := range volumes {
		ss += describeVolume(volumes[i])
		if i < len(volumes)-1 {
			ss += ","
		}
	}
	ss += "]"
	return ss, nil
}

func volumeMountsFn(data interface{}) (string, error) {
	var mounts []core.VolumeMount
	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &mounts)
		if err != nil {
			return "", err
		}
	} else if _, ok := data.([]interface{}); ok {
		err := meta_util.DecodeObject(data, &mounts)
		if err != nil {
			return "", err
		}
	}

	ss := make([]string, 0, len(mounts))
	for i := range mounts {
		mnt := fmt.Sprintf("%s:%s", mounts[i].Name, mounts[i].MountPath)
		if mounts[i].SubPath != "" {
			mnt = fmt.Sprintf("%s:%s:%s", mounts[i].Name, mounts[i].MountPath, mounts[i].SubPath)
		}
		ss = append(ss, mnt)
	}
	return strings.Join(ss, "\n"), nil
}

func fmtListFn(data interface{}) (string, error) {
	if s, ok := data.(string); ok && s != "" {
		return s, nil
	} else if arr, ok := data.([]interface{}); ok && len(arr) > 0 {
		s, err := json.Marshal(arr)
		return string(s), err
	}
	// Return empty list if the data is empty. This helps to avoid object parsing error.
	// ref: https://stackoverflow.com/a/18419503
	return "[]", nil
}

type ObjectNamespace struct {
	Namespace string `json:"namespace,omitempty"`
}

type promNamespaceSelector struct {
	ObjectNamespace `json:"metadata"`
	Spec            promNamespaceSelectorSpec `json:"spec"`
}

type promNamespaceSelectorSpec struct {
	NamespaceSelector *prom_op.NamespaceSelector `json:"namespaceSelector,omitempty"`
}

func promNamespaceSelectorFn(data interface{}) (string, error) {
	var obj promNamespaceSelector
	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &obj)
		if err != nil {
			return "", err
		}
	} else if _, ok := data.(map[string]interface{}); ok {
		err := meta_util.DecodeObject(data, &obj)
		if err != nil {
			return "", err
		}
	}

	// If selector field is empty, then all namespaces are the targets.
	if obj.Spec.NamespaceSelector == nil {
		return "All", nil
	} else if len(obj.Spec.NamespaceSelector.MatchNames) != 0 {
		// If an array of namespace is provided, then those namespaces are the target
		return strings.Join(obj.Spec.NamespaceSelector.MatchNames, ", "), nil
	} else if !obj.Spec.NamespaceSelector.Any {
		// If "any: false" is set in the namespace selector field, only the object namespace is the target.
		return obj.Namespace, nil
	}
	return "", nil
}

func containerImagesFn(data interface{}) (string, error) {
	var containers []core.Container
	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &containers)
		if err != nil {
			return "", err
		}
	} else if _, ok := data.(map[string]interface{}); ok {
		err := meta_util.DecodeObject(data, &containers)
		if err != nil {
			return "", err
		}
	}

	var imagesBuffer bytes.Buffer
	for i, container := range containers {
		imagesBuffer.WriteString(container.Image)
		if i != len(containers)-1 {
			imagesBuffer.WriteString(",")
		}
	}
	return imagesBuffer.String(), nil
}

func durationFn(start interface{}, end ...interface{}) (string, error) {
	var st metav1.Time
	if s, ok := start.(string); ok && s != "" {
		err := st.UnmarshalQueryParameter(s)
		if err != nil {
			return "", err
		}
	} else if v, ok := start.(metav1.Time); ok {
		st = v
	}

	if len(end) == 0 || end[0] == nil {
		// only start time exists
		return duration.HumanDuration(time.Since(st.Time)), nil
	}

	var et metav1.Time
	if s, ok := end[0].(string); ok && s != "" {
		err := et.UnmarshalQueryParameter(s)
		if err != nil {
			return "", err
		}
	} else if v, ok := end[0].(metav1.Time); ok {
		et = v
	}
	return duration.HumanDuration(et.Sub(st.Time)), nil
}

func mapKeyCountFn(data interface{}) (string, error) {
	var m map[string]interface{}

	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &m)
		if err != nil {
			return "", err
		}
	} else if v, ok := data.(map[string]interface{}); ok {
		m = v
	}

	if m == nil {
		return UnknownValue, nil
	}
	return strconv.Itoa(len(m)), nil
}

func rbacSubjects(data interface{}) (string, error) {
	var subjects []rbac.Subject
	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &subjects)
		if err != nil {
			return "", err
		}
	} else if _, ok := data.([]interface{}); ok {
		err := meta_util.DecodeObject(data, &subjects)
		if err != nil {
			return "", err
		}
	}

	ss := make([]string, 0, len(subjects))
	for i := range subjects {
		s := fmt.Sprintf("%s %s", subjects[i].Kind, subjects[i].Name)
		if subjects[i].Namespace != "" {
			s = fmt.Sprintf("%s %s/%s", subjects[i].Kind, subjects[i].Namespace, subjects[i].Name)
		}
		ss = append(ss, s)
	}
	return strings.Join(ss, ","), nil
}

func certificateValidity(data interface{}) (string, error) {
	certStatus := struct {
		NotBefore metav1.Time `json:"notBefore"`
		NotAfter  metav1.Time `json:"notAfter"`
	}{}
	if s, ok := data.(string); ok && s != "" {
		err := json.Unmarshal([]byte(s), &certStatus)
		if err != nil {
			return "", err
		}
	} else if _, ok := data.(map[string]interface{}); ok {
		err := meta_util.DecodeObject(data, &certStatus)
		if err != nil {
			return "", err
		}
	}

	now := time.Now()
	if certStatus.NotBefore.IsZero() || certStatus.NotAfter.IsZero() {
		return UnknownValue, nil
	} else if certStatus.NotBefore.After(now) {
		return "Not valid yet", nil
	} else if now.After(certStatus.NotAfter.Time) {
		return "Expired", nil
	}
	return duration.HumanDuration(time.Until(certStatus.NotAfter.Time)), nil
}

func formatResourceCPUFn(data interface{}) (string, error) {
	var cpu string
	if s, ok := data.(string); ok && s != "" {
		if strings.HasSuffix(s, "m") {
			cpu = s[:len(s)-1]
			c, err := strconv.Atoi(cpu)
			if err != nil {
				return "", err
			}
			cpu = fmt.Sprintf("%v", float64(c)/1000.0)
		} else {
			cpu = s
		}
	}
	return cpu, nil
}

func formatResourceMemoryFn(data interface{}) (string, error) {
	if s, ok := data.(string); !ok || s == "" {
		return "", nil
	}
	mem, err := convertSizeToBytes(data.(string))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%.1fGi", mem/1024.0/1024.0/1024.0), nil
}
