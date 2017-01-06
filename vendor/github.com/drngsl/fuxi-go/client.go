package fuxi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

const VolumeNotFound = "Volume Not Found"

type Fuxi interface {
	Activate() (implements []string, err error)
	Create(name string, opts map[string]string) (err error)
	Remove(name string) (err error)
	Path(name string) (mountpoint string, err error)
	Mount(name, id string) (mountpoint string, err error)
	List() (volumes []*FuxiVolume, err error)
	Get(name string) (volume *FuxiVolume, err error)
	Capabilities() (capabilities Capability, err error)
}

type FuxiVolume struct {
	Name		string
	Mountpoint	string
	Status          map[string]interface{}
}

type Client struct {
	*http.Client

	schema  string
	host    string
	port    int
}

func NewClient(host string, port int) (*Client, error) {
	client := &http.Client{}

	return &Client{
		Client:      client,
		schema:      "http",
		host:        host,
		port:        port,
	}, nil
}

func (c Client) request(method, url string, request interface{}, response interface{}) error {
	var (
		b   []byte
		err error
	)

	if method == "POST" {
		b, err = json.Marshal(request)
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}

	return nil
}

func (c Client) post(url string, request interface{}, response interface{}) error {
	return c.request("POST", url, request, response)
}

func (c Client) getURL(path string) string {
	return fmt.Sprintf("%s://%s:%d/%s", c.schema, c.host, c.port, path)
}

type ActivateRequest struct {
}

type ActiveteResponse struct {
	Implements	[]string
}

func (c Client) Activate() (implements []string, err error) {
	var (
		req	ActivateRequest
		resp	ActiveteResponse
	)
	if err = c.post(c.getURL("Plugin.Activate"), req, &resp); err != nil{
		return
	}
	implements = resp.Implements
	return
}

type CreateRequest struct {
	Name	string
	Opts	map[string]string
}

type CreateResponse struct {
	Err	string
}

func (c Client) Create(volName string, opts map[string]string) (err error) {
	var (
		req	CreateRequest
		resp	CreateResponse
	)
	req.Name = volName
	req.Opts = opts

	if err = c.post(c.getURL("VolumeDriver.Create"), req, &resp); err != nil{
		return
	}
	if resp.Err != "" {
		err = errors.New(resp.Err)
	}
	return
}

type RemoveRequest struct {
	Name	string
}

type RemoveResponse struct {
	Err	string
}

func (c Client) Remove(volName string) (err error)  {
	var (
		req	RemoveRequest
		resp	RemoveResponse

	)
	req.Name = volName
	if err = c.post(c.getURL("VolumeDriver.Remove"), req, &resp); err != nil {
		return
	}
	if resp.Err != "" {
		err = errors.New(resp.Err)
	}
	return
}

type MountRequest struct {
	Name		string
}

type MountResponse struct  {
	Mountpoint	string
	Err		string
}

func (c Client) Mount(volName string, ID string) (mountpoint string, err error) {
	var (
		req	MountRequest
		resp	MountResponse
	)
	req.Name = volName
	if err = c.post(c.getURL("VolumeDriver.Mount"), req, &resp); err != nil {
		return
	}

	mountpoint = resp.Mountpoint

	if resp.Err != "" {
		err = errors.New(resp.Err)
	}
	return
}

type PathRequest struct {
	Name		string
}

type PathResponse struct {
	Mountpoint	string
	Err		string
}

func (c Client) Path(volName string) (mountpoint string, err error) {
	var (
		req	PathRequest
		resp	PathResponse
	)
	req.Name = volName
	if err = c.post(c.getURL("VolumeDriver.Get"), req, &resp); err != nil {
		return
	}

	mountpoint = resp.Mountpoint

	if resp.Err != "" {
		err = errors.New(resp.Err)
	}
	return
}

type GetRequest	struct {
	Name		string
}

type GetResponse struct {
	Volume		FuxiVolume
	Err		string
}
func (c Client) Get(volName string) (volume *FuxiVolume, err error) {
	var (
		req	GetRequest
		resp	GetResponse
	)
	req.Name = volName
	if err = c.post(c.getURL("VolumeDriver.Get"), req, &resp); err != nil {
		return
	}

	volume = &resp.Volume

	if resp.Err != "" {
		err = errors.New(resp.Err)
	}

	return
}

type ListRequest struct {
}

type ListResponse struct  {
	Volumes		[]*FuxiVolume
	Err		string

}
func (c Client) List() (volumes []*FuxiVolume, err error) {
	var (
		req	ListRequest
		resp	ListResponse
	)
	if err = c.post(c.getURL("VolumeDriver.List"), req, &resp); err != nil {
		return
	}

	volumes = resp.Volumes

	if resp.Err != "" {
		err = errors.New(resp.Err)
	}
	return
}

type Capability struct {
	Scope string
}

type CapabilitiesRequest struct {
}

type CapabilitiesResponse struct {
	Capabilities Capability
	Err          string
}

func (c Client) Capabilities() (capabilities Capability, err error) {
	var (
		req CapabilitiesRequest
		ret CapabilitiesResponse
	)

	if err = c.post(c.getURL("VolumeDriver.Capabilities"), req, &ret); err != nil {
		return
	}

	capabilities = ret.Capabilities

	if ret.Err != "" {
		err = errors.New(ret.Err)
	}
	return
}
