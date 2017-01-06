/*
Copyright 2016 The Kubernetes Authors.

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

package fuxi

import (
	"fmt"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util/env"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume"

	fuxiclient "github.com/drngsl/fuxi-go"
)

const (
	fuxiVolumePluginName = "kubernetes.io/fuxi"
	defaultHost          = "localhost"
	defaultPort          = 7879
)

// ProbeVolumePlugins is the primary entrypoint for volume plugins.
func ProbeVolumePlugins() []volume.VolumePlugin {
	return []volume.VolumePlugin{&fuxiPlugin{nil}}
}

type fuxiPlugin struct {
	host volume.VolumeHost
}

var _ volume.VolumePlugin = &fuxiPlugin{}
var _ volume.PersistentVolumePlugin = &fuxiPlugin{}

func (plugin *fuxiPlugin) Init(host volume.VolumeHost) error {
	plugin.host = host
	return nil
}

func (plugin *fuxiPlugin) GetPluginName() string {
	return fuxiVolumePluginName
}

func (plugin *fuxiPlugin) GetVolumeName(spec *volume.Spec) (string, error) {
	volumeSource, _, err := getVolumeSource(spec)
	if err != nil {
		return "", err
	}

	return volumeSource.Volume, nil
}

func (plugin *fuxiPlugin) CanSupport(spec *volume.Spec) bool {
	return (spec.Volume != nil && spec.Volume.Fuxi != nil) || (spec.PersistentVolume != nil && spec.PersistentVolume.Spec.Fuxi != nil)
}

func (plugin *fuxiPlugin) RequiresRemount() bool {
	return false
}

func (plugin *fuxiPlugin) GetAccessModes() []api.PersistentVolumeAccessMode {
	return []api.PersistentVolumeAccessMode{
		api.ReadWriteOnce,
		api.ReadOnlyMany,
		api.ReadWriteMany,
	}
}

func getVolumeSource(spec *volume.Spec) (*api.FuxiVolumeSource, bool, error) {
	if spec.Volume != nil && spec.Volume.Fuxi != nil {
		return spec.Volume.Fuxi, false, nil
	} else if spec.PersistentVolume != nil &&
		spec.PersistentVolume.Spec.Fuxi != nil {
		return spec.PersistentVolume.Spec.Fuxi, spec.ReadOnly, nil
	}

	return nil, false, fmt.Errorf("Spec does not reference a Fuxi volume type")
}

func (plugin *fuxiPlugin) ConstructVolumeSpec(volumeName, mountPath string) (*volume.Spec, error) {
	fuxiVolume := &api.Volume{
		Name: volumeName,
		VolumeSource: api.VolumeSource{
			Fuxi: &api.FuxiVolumeSource{
				Volume: volumeName,
			},
		},
	}
	return volume.NewSpecFromVolume(fuxiVolume), nil
}

func (plugin *fuxiPlugin) NewMounter(spec *volume.Spec, pod *api.Pod, _ volume.VolumeOptions) (volume.Mounter, error) {
	return plugin.newMounterInternal(spec, pod, plugin.host.GetMounter())
}

func (plugin *fuxiPlugin) newMounterInternal(spec *volume.Spec, pod *api.Pod, mounter mount.Interface) (volume.Mounter, error) {
	source, readOnly, err := getVolumeSource(spec)
	if err != nil {
		return nil, err
	}

	return &fuxiMounter{
		fuxi: &fuxi{
			mounter: mounter,
			pod:     pod,
			volume:  source.Volume,
			plugin:  plugin,
		},
		options: source.Options,
		readOnly: readOnly}, nil
}

func (plugin *fuxiPlugin) NewUnmounter(volName string, podUID types.UID) (volume.Unmounter, error) {
	return plugin.newUnmounterInternal(volName, podUID, plugin.host.GetMounter())
}

func (plugin *fuxiPlugin) newUnmounterInternal(volName string, podUID types.UID, mounter mount.Interface) (volume.Unmounter, error) {
	return &fuxiUnmounter{&fuxi{
		mounter: mounter,
		pod:     &api.Pod{ObjectMeta: api.ObjectMeta{UID: podUID}},
		volume:  volName,
		plugin:  plugin,
	}}, nil
}

type fuxi struct {
	pod     *api.Pod
	volume  string
	path    string
	mounter mount.Interface
	plugin  *fuxiPlugin
	volume.MetricsNil
}

type fuxiMounter struct {
	*fuxi
	options  map[string]string
	client	 fuxiclient.Fuxi
	readOnly bool
}

var _ volume.Mounter = &fuxiMounter{}

func (fuxiVolume *fuxi) NewFuxiClient() (*fuxiclient.Client, error) {
	host, err := fuxiVolume.plugin.host.GetHostIP()
	if err != nil {
		return nil, err
	}
	port, err := env.GetEnvAsIntOrFallback("FUXI_SERVER_PORT", defaultPort)
	if err != nil {
		return nil, err
	}
	return fuxiclient.NewClient(host.String(), port)
}

func (mounter *fuxiMounter) CanMount() error {
        return nil
}

func (mounter *fuxiMounter) GetAttributes() volume.Attributes {
	return volume.Attributes{
		ReadOnly:        mounter.readOnly,
		Managed:         false,
		SupportsSELinux: false,
	}
}

// SetUp attaches the disk and bind mounts to the volume path.
func (mounter *fuxiMounter) SetUp(fsGroup *int64) error {
	return mounter.SetUpAt(mounter.fuxi.volume, fsGroup)
}

func (mounter *fuxiMounter) SetUpAt(dir string, fsGroup *int64) error {
	if mounter.client == nil {
		c, err := mounter.NewFuxiClient()
		if err != nil {
			return err
		}
		mounter.client = c
	}
	vol, err := mounter.client.Get(dir)
	if err != nil {
		if err.Error() == fuxiclient.VolumeNotFound {
			if mounter.options == nil {
				mounter.options = make(map[string]string)
			}
			if err = mounter.client.Create(dir, mounter.options); err != nil {
				return err
			}
			vol, err = mounter.client.Get(dir)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if vol.Mountpoint != "" {
		mounter.fuxi.path = vol.Mountpoint
	} else {
		mountpoint, err := mounter.client.Mount(dir, "")
		if err != nil {
			return err
		}
		mounter.fuxi.path = mountpoint
	}
	return nil
}

func (fuxiVolume *fuxi) GetPath() string {
	if fuxiVolume.path != "" {
		return fuxiVolume.path
	}
	c, err := fuxiVolume.NewFuxiClient()
	if err != nil {
		return ""
	}
	vol, err := c.Get(fuxiVolume.volume)
	if err != nil {
		return ""
	}
	return vol.Mountpoint
}

type fuxiUnmounter struct {
	*fuxi
}

var _ volume.Unmounter = &fuxiUnmounter{}

func (unmounter *fuxiUnmounter) TearDown() error {
	return unmounter.TearDownAt(unmounter.GetPath())
}

func (unmounter *fuxiUnmounter) TearDownAt(dir string) error {
	return nil
}
