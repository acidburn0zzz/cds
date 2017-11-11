package objectstore

import (
	"fmt"
	"io"
	"time"

	"github.com/ovh/cds/engine/api/sessionstore"

	"github.com/ncw/swift"
	"github.com/ovh/cds/sdk"
	"github.com/ovh/cds/sdk/log"
)

// SwiftStore implements ObjectStore interface with openstack swift implementation
type SwiftStore struct {
	swift.Connection
	containerprefix string
}

// NewSwiftStore create a new ObjectStore with openstack driver and check configuration
func NewSwiftStore(authURL, user, password, region, tenant, containerprefix string) (Driver, error) {
	s := SwiftStore{
		swift.Connection{
			AuthUrl:  authURL,
			Region:   region,
			Tenant:   tenant,
			UserName: user,
			ApiKey:   password,
		}, containerprefix}

	if err := s.Authenticate(); err != nil {
		return nil, sdk.WrapError(err, "Swift> Unable to authenticate")
	}
	return &s, nil
}

// Status returns the status of swift account
func (s *SwiftStore) Status() string {
	info, _, err := s.Account()
	if err != nil {
		return "Swift KO: " + err.Error()
	}
	return fmt.Sprintf("Swift OK (%d containers, %d objects, %d bytes used", info.Containers, info.Containers, info.BytesUsed)
}

// Store stores in swift
func (s *SwiftStore) Store(o Object, data io.ReadCloser) (string, error) {
	container := s.containerprefix + o.GetPath()
	object := o.GetName()
	escape(container, object)
	log.Debug("SwiftStore> Storing /%s/%s\n", container, object)

	log.Debug("SwiftStore> creating container %s", container)
	if err := s.ContainerCreate(container, nil); err != nil {
		return "", sdk.WrapError(err, "SwiftStore> Unable to create container %s", container)
	}

	log.Debug("SwiftStore> creating object %s/%s", container, object)

	file, errC := s.ObjectCreate(container, object, false, "", "application/octet-stream", nil)
	if errC != nil {
		return "", sdk.WrapError(errC, "SwiftStore> Unable to create object %s", object)
	}

	log.Debug("SwiftStore> copy object %s/%s", container, object)
	if _, err := io.Copy(file, data); err != nil {
		return "", sdk.WrapError(err, "SwiftStore> Unable to copy object buffer %s", object)
	}

	if err := file.Close(); err != nil {
		return "", sdk.WrapError(err, "SwiftStore> Unable to close object buffer %s", object)
	}

	if err := data.Close(); err != nil {
		return "", sdk.WrapError(err, "SwiftStore> Unable to close data buffer")
	}

	return container + "/" + object, nil
}

func (s *SwiftStore) Fetch(o Object) (io.ReadCloser, error) {
	container := s.containerprefix + o.GetPath()
	object := o.GetName()
	escape(container, object)

	pipeReader, pipeWriter := io.Pipe()
	log.Debug("OpenstacSwiftStorekStore> Fetching /%s/%s\n", container, object)

	go func() {
		log.Debug("SwiftStore> downloading object %s%s", container, object)

		if _, err := s.ObjectGet(container, object, pipeWriter, false, nil); err != nil {
			log.Error("SwiftStore> Unable to get object %s/%s", container, object)
		}

		log.Debug("SwiftStore> object %s%s downloaded", container, object)
		pipeWriter.Close()
	}()
	return pipeReader, nil
}

func (s *SwiftStore) Delete(o Object) error {
	container := s.containerprefix + o.GetPath()
	object := o.GetName()
	escape(container, object)

	if err := s.ObjectDelete(container, object); err != nil {
		return sdk.WrapError(err, "SwiftStore> Unable to delete object")
	}
	return nil
}

func (s *SwiftStore) StoreURL(o Object) (string, string, error) {
	container := s.containerprefix + o.GetPath()
	object := o.GetName()
	escape(container, object)

	if err := s.ContainerCreate(container, nil); err != nil {
		return "", "", sdk.WrapError(err, "SwiftStore> Unable to create container %s", container)
	}

	key, _ := sessionstore.NewSessionKey()
	url := s.ObjectTempUrl(container, object, string(key), "PUT", time.Now().Add(15*time.Minute))
	return url, string(key), nil
}

func (s *SwiftStore) FetchURL(o Object) (string, string, error) {
	container := s.containerprefix + o.GetPath()
	object := o.GetName()
	escape(container, object)
	key, _ := sessionstore.NewSessionKey()
	url := s.ObjectTempUrl(container, object, string(key), "GET", time.Now().Add(15*time.Minute))
	return url, string(key), nil
}