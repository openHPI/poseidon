package storage

import (
	"context"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/pkg/monitoring"
)

// Storage is an interface for storing objects.
type Storage[T any] interface {
	// List returns all objects from the storage.
	List() []T

	// Add adds an object to the storage.
	// It overwrites the old object if one with the same id was already stored.
	Add(id string, o T)

	// Get returns an object from the storage.
	// Iff the object does not exist in the storage, ok will be false.
	Get(id string) (o T, ok bool)

	// Delete deletes the object with the passed id from the storage.
	// It does nothing if no object with the id is present in the store.
	Delete(id string)

	// Pop deletes the object with the given id from the storage and returns it.
	// Iff no such execution exists, ok is false and true otherwise.
	Pop(id string) (o T, ok bool)

	// Purge removes all objects from the storage.
	Purge()

	// Length returns the number of currently stored objects in the storage.
	Length() uint

	// Sample returns and removes an arbitrary object from the storage.
	// ok is true iff an object was returned.
	Sample() (o T, ok bool)
}

// EventType is an enum type to declare the different causes of a monitoring event.
type EventType string

const (
	Creation     EventType = "creation"
	Deletion     EventType = "deletion"
	Periodically EventType = "periodically"
)

// WriteCallback is called before an event gets monitored.
// Iff eventType is Periodically it is no object provided.
type WriteCallback[T any] func(p *write.Point, object T, eventType EventType)

// localStorage stores objects in the local application memory.
type localStorage[T any] struct {
	sync.RWMutex
	objects     map[string]T
	measurement string
	callback    WriteCallback[T]
}

// NewLocalStorage responds with a Storage implementation.
// This implementation stores the data thread-safe in the local application memory.
func NewLocalStorage[T any]() *localStorage[T] {
	return &localStorage[T]{
		objects: make(map[string]T),
	}
}

// NewMonitoredLocalStorage responds with a Storage implementation.
// All write operations are monitored in the passed measurement.
// Iff callback is set, it will be called on a write operation.
// Iff additionalEvents not zero, the duration will be used to periodically send additional monitoring events.
func NewMonitoredLocalStorage[T any](
	measurement string, callback WriteCallback[T], additionalEvents time.Duration, ctx context.Context,
) *localStorage[T] {
	s := &localStorage[T]{
		objects:     make(map[string]T),
		measurement: measurement,
		callback:    callback,
	}
	if additionalEvents != 0 {
		go s.periodicallySendMonitoringData(additionalEvents, ctx)
	}
	return s
}

func (s *localStorage[T]) List() (o []T) {
	s.RLock()
	defer s.RUnlock()
	for _, value := range s.objects {
		o = append(o, value)
	}
	return o
}

func (s *localStorage[T]) Add(id string, o T) {
	s.Lock()
	defer s.Unlock()
	s.objects[id] = o
	s.sendMonitoringData(id, o, Creation, s.unsafeLength())
}

func (s *localStorage[T]) Get(id string) (o T, ok bool) {
	s.RLock()
	defer s.RUnlock()
	o, ok = s.objects[id]
	return
}

func (s *localStorage[T]) Delete(objectID string) {
	s.Lock()
	defer s.Unlock()
	o, ok := s.objects[objectID]
	if ok {
		delete(s.objects, objectID)
		s.sendMonitoringData(objectID, o, Deletion, s.unsafeLength())
	}
}

func (s *localStorage[T]) Pop(id string) (T, bool) {
	o, ok := s.Get(id)
	s.Delete(id)
	return o, ok
}

func (s *localStorage[T]) Purge() {
	s.Lock()
	defer s.Unlock()
	for key, object := range s.objects {
		s.sendMonitoringData(key, object, Deletion, 0)
	}
	s.objects = make(map[string]T)
}

func (s *localStorage[T]) Sample() (o T, ok bool) {
	s.Lock()
	defer s.Unlock()
	for key, object := range s.objects {
		delete(s.objects, key)
		s.sendMonitoringData(key, object, Deletion, s.unsafeLength())
		return object, true
	}
	return o, false
}

func (s *localStorage[T]) Length() uint {
	s.RLock()
	defer s.RUnlock()
	return s.unsafeLength()
}

func (s *localStorage[T]) unsafeLength() uint {
	length := len(s.objects)
	return uint(length)
}

func (s *localStorage[T]) sendMonitoringData(id string, object T, eventType EventType, count uint) {
	if s.measurement != "" {
		dataPoint := influxdb2.NewPointWithMeasurement(s.measurement)
		dataPoint.AddTag("id", id)
		dataPoint.AddTag("event_type", string(eventType))
		dataPoint.AddField("count", count)

		if s.callback != nil {
			s.callback(dataPoint, object, eventType)
		}

		monitoring.WriteInfluxPoint(dataPoint)
	}
}

func (s *localStorage[T]) periodicallySendMonitoringData(d time.Duration, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
			stub := new(T)
			s.sendMonitoringData("", *stub, Periodically, s.Length())
		}
	}
}
