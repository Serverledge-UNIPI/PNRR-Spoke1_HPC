package containers

import (
	"bytes"
	"container/list"
	"encoding/base64"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/grussorusso/serverledge/internal/config"

	"github.com/grussorusso/serverledge/internal/functions"
)

var node NodeResources

func Initialize() {
	// initialize node resources
	availableCores := runtime.NumCPU()
	node.AvailableMemMB = int64(config.GetInt(config.POOL_MEMORY_MB, 1024))
	node.AvailableCPUs = config.GetFloat(config.POOL_CPUS, float64(availableCores)*2.0)
	node.containerPools = make(map[string]*containerPool)
	log.Printf("Current node resources: %v", node)

	InitDockerContainerFactory()

	//janitor periodically remove expired warm container
	GetJanitorInstance()
}

//getFunctionPool retrieves (or creates) the container pool for a function.
func getFunctionPool(f *functions.Function) *containerPool {
	if fp, ok := node.containerPools[f.Name]; ok {
		return fp
	}

	fp := newFunctionPool(f)
	node.containerPools[f.Name] = fp
	return fp
}

func (fp *containerPool) acquireReadyContainer() (ContainerID, bool) {
	// TODO: picking most-recent / least-recent container might be better?
	elem := fp.ready.Front()
	if elem == nil {
		return "", false
	}

	fp.ready.Remove(elem)
	contID := elem.Value.(warmContainer).contID
	fp.putBusyContainer(contID)

	return contID, true
}

func (fp *containerPool) putBusyContainer(contID ContainerID) {
	fp.busy.PushBack(contID)
}

func (fp *containerPool) putReadyContainer(contID ContainerID, expiration int64) {
	fp.ready.PushBack(warmContainer{
		contID:     contID,
		Expiration: expiration,
	})
}

func newFunctionPool(f *functions.Function) *containerPool {
	fp := &containerPool{}
	fp.busy = list.New()
	fp.ready = list.New()

	return fp
}

// AcquireWarmContainer acquires a warm container for a given function (if any).
// A warm container is in running/paused state and has already been initialized
// with the function code.
// The acquired container is already in the busy pool.
// The function returns an error if either:
// (i) the warm container does not exist
// (ii) there are not enough resources to start the container
func AcquireWarmContainer(f *functions.Function) (ContainerID, error) {
	node.Lock()
	defer node.Unlock()

	if node.AvailableCPUs < f.CPUDemand {
		log.Printf("Not enough CPU to start a warm container for %s", f)
		return "", OutOfResourcesErr
	}

	fp := getFunctionPool(f)
	fp.Lock()
	defer fp.Unlock()

	contID, found := fp.acquireReadyContainer()
	if found {
		node.AvailableCPUs -= f.CPUDemand
		log.Printf("Acquired resources for warm container. Now: %v", node)
		return contID, nil
	}

	return "", NoWarmFoundErr
}

// ReleaseContainer puts a container in the ready pool for a function.
func ReleaseContainer(contID ContainerID, f *functions.Function) {
	//time.Sleep(15 * time.Second)
	log.Printf("Container released for %v: %v", f, contID)

	node.Lock()
	defer node.Unlock()

	fp := getFunctionPool(f)
	fp.Lock()
	defer fp.Unlock()

	// setup Expiration as time duration from now
	//todo adjust default value
	d := time.Duration(config.GetInt("container.expiration", 30)) * time.Second
	fp.putReadyContainer(contID, time.Now().Add(d).UnixNano())

	// we must update the busy list by removing this element
	elem := fp.busy.Front()
	for ok := elem != nil; ok; ok = elem != nil {
		if elem.Value.(ContainerID) == contID {
			fp.busy.Remove(elem) // delete the element from the busy list
			break
		}
		elem.Next()
	}

	node.AvailableCPUs += f.CPUDemand

	log.Printf("Released resources. Now: %v", node)
}

//NewContainer creates and starts a new container for the given function.
//The container can be directly used to schedule a request, as it is already
//in the busy pool.
func NewContainer(fun *functions.Function) (ContainerID, error) {
	runtime, ok := runtimeToInfo[fun.Runtime]
	if !ok {
		return "", fmt.Errorf("Invalid runtime: %s", fun.Runtime)
	}
	image := runtime.Image

	node.Lock()
	// check resources
	if node.AvailableMemMB < fun.MemoryMB {
		enoughMem, _ := dismissContainer(fun.MemoryMB)
		if !enoughMem {
			node.Unlock()
			log.Printf("Not enough memory for the new container.")
			return "", OutOfResourcesErr
		}
	}
	if node.AvailableCPUs < fun.CPUDemand {
		node.Unlock()
		log.Printf("Not enough CPU for the new container.")
		return "", OutOfResourcesErr
	}

	node.AvailableMemMB -= fun.MemoryMB
	node.AvailableCPUs -= fun.CPUDemand
	fp := getFunctionPool(fun)
	node.Unlock()

	log.Printf("Acquired resources for new container. Now: %v", node)

	log.Printf("Starting new container for %s (image: %s)", fun, image)
	contID, err := cf.Create(image, &ContainerOptions{
		MemoryMB: fun.MemoryMB,
	})
	if err != nil {
		log.Printf("Failed container creation")
		return "", err
	}

	decodedCode, _ := base64.StdEncoding.DecodeString(fun.TarFunctionCode)
	err = cf.CopyToContainer(contID, bytes.NewReader(decodedCode), "/app/")
	if err != nil {
		log.Printf("Failed code copy")
		return "", err
	}

	err = cf.Start(contID)
	if err != nil {
		return "", err
	}

	fp.Lock()
	defer fp.Unlock()
	fp.putBusyContainer(contID) // We immediately mark it as busy

	return contID, nil
}

type itemToDismiss struct {
	contID ContainerID
	pool   *containerPool
	elem   *list.Element
	memory int64
}

// dismissContainer ... this function is used to get free memory used for a new container
// 2-phases: first, we find ready containers and collect them as a slice, second (cleanup phase) we delete the containers only and only if
// the sum of their memory is >= requiredMemoryMB is
func dismissContainer(requiredMemoryMB int64) (bool, error) {
	node.Lock()
	defer node.Unlock()

	var cleanedMB int64 = 0
	var containerToDismiss []itemToDismiss
	var toUnlock []*containerPool
	res := false

	//first phase, research
	for _, funPool := range node.containerPools {
		funPool.Lock()
		if funPool.ready.Len() > 0 {
			toUnlock = append(toUnlock, funPool)
			// every container into the funPool has the same memory (same function)
			//so it is not important which one you destroy
			elem := funPool.ready.Front()
			contID := elem.Value.(warmContainer).contID
			// containers in the same pool need same memory
			memory, _ := cf.GetMemoryMB(contID)
			for ok := true; ok; ok = elem != nil {
				containerToDismiss = append(containerToDismiss,
					itemToDismiss{contID: contID, pool: funPool, elem: elem, memory: memory})

				cleanedMB += memory
				if cleanedMB >= requiredMemoryMB {
					goto cleanup
				}
				//go on to the next one
				elem = elem.Next()
			}
		} else {
			// ready list is empty
			funPool.Unlock()
		}
	}

cleanup: // second phase, cleanup
	// memory check
	if cleanedMB >= requiredMemoryMB {
		for _, item := range containerToDismiss {
			item.pool.ready.Remove(item.elem) // remove the container from the funPool
			err := cf.Destroy(item.contID)    // destroy the container
			if err != nil {
				res = false
				goto unlock
			}
			node.AvailableMemMB += item.memory
		}

		res = true
		log.Printf("Released resources. Now: %v", node)
	}

unlock:
	for _, elem := range toUnlock {
		elem.Unlock()
	}

	return res, nil
}

// DeleteExpiredContainer is called by the container janitor
// Deletes expired warm containers
func DeleteExpiredContainer() {
	now := time.Now().UnixNano()

	node.Lock()
	defer node.Unlock()

	for _, pool := range node.containerPools {
		pool.Lock()

		elem := pool.ready.Front()
		for ok := elem != nil; ok; ok = elem != nil {
			warmed := elem.Value.(warmContainer)
			if now > warmed.Expiration {
				temp := elem
				elem = elem.Next()
				log.Printf("janitor: Removing container with ID %s\n", warmed.contID)
				pool.ready.Remove(temp) // remove the expired element

				memory, _ := cf.GetMemoryMB(warmed.contID)
				cf.Destroy(warmed.contID)
				node.AvailableMemMB += memory
				log.Printf("Released resources. Now: %v", node)

			} else {
				elem = elem.Next()
			}
		}

		pool.Unlock()
	}

}

// Destroys all containers (usually on termination)
func ShutdownAll() {
	node.Lock()
	defer node.Unlock()

	for fun, pool := range node.containerPools {
		pool.Lock()

		elem := pool.ready.Front()
		for ok := elem != nil; ok; ok = elem != nil {
			warmed := elem.Value.(warmContainer)
			temp := elem
			elem = elem.Next()
			log.Printf("Removing container with ID %s\n", warmed.contID)
			pool.ready.Remove(temp)

			memory, _ := cf.GetMemoryMB(warmed.contID)
			cf.Destroy(warmed.contID)
			node.AvailableMemMB += memory
		}

		function, _ := functions.GetFunction(fun)

		elem = pool.busy.Front()
		for ok := elem != nil; ok; ok = elem != nil {
			contID := elem.Value.(ContainerID)
			temp := elem
			elem = elem.Next()
			log.Printf("Removing container with ID %s\n", contID)
			pool.ready.Remove(temp)

			memory, _ := cf.GetMemoryMB(contID)
			cf.Destroy(contID)
			node.AvailableMemMB += memory
			node.AvailableCPUs += function.CPUDemand
		}

		pool.Unlock()
	}
}
