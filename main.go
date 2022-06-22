package main

import (
	"context"
	"fmt"
	"log"
	"syscall"
	"time"

	"github.com/bithavoc/test-containerd/cninetwork"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/services/tasks/v1"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
)

func main() {
	if err := redisExample(); err != nil {
		log.Fatal(err)
	}
}

func redisExample() error {
	cni, err := cninetwork.InitNetwork()
	if err != nil {
		return fmt.Errorf("failed to init cni, %w", err)
	}
	log.Printf("CNI initialized")
	// create a new client connected to the default socket path for containerd
	client, err := containerd.New("/run/containerd/containerd.sock")
	if err != nil {
		return err
	}
	defer client.Close()

	// create a new context with an "example" namespace
	ctx := namespaces.WithNamespace(context.Background(), "default")

	// pull the redis image from DockerHub
	image, err := client.Pull(ctx, "docker.io/library/ubuntu:latest", containerd.WithPullUnpack)
	if err != nil {
		return err
	}
	images, err := client.ListImages(ctx)
	if err != nil {
		return err
	}
	log.Printf("images before")
	for _, img := range images {
		log.Printf("Image %s", img.Name())
	}
	// if err := client.ImageService().Delete(ctx, "docker.io/library/redis:alpine"); err != nil {
	// 	return err
	// }
	// log.Printf("images after delete")
	images, err = client.ListImages(ctx)
	if err != nil {
		return err
	}
	for _, img := range images {
		log.Printf("Image %s", img.Name())
	}
	//  client.ContentStore().ListStatuses()

	// create a container
	containerName := "ubuntu-server2"
	_, err = client.ContainerService().Get(ctx, containerName)
	if errdefs.IsNotFound(err) {

	} else if err != nil {
		return fmt.Errorf("failed to retrieve container by id, %w, %T", err, err)
	} else {
		log.Printf("deleting container")
		client.ContainerService().Delete(ctx, containerName)
	}
	snapshooter := ""
	snapshotName := containerName + "-snapshot"
	client.SnapshotService(snapshooter).Remove(ctx, snapshotName)
	container, err := client.NewContainer(
		ctx,
		containerName,
		containerd.WithImage(image),
		containerd.WithSnapshotter(snapshooter),
		containerd.WithNewSnapshot(snapshotName, image),
		containerd.WithNewSpec(
			oci.WithImageConfig(image),
			oci.WithMemoryLimit(((1024*1024)*4)),
			oci.WithProcessArgs("sh", "-c", "apt update"),
		),
	)
	if err != nil {
		return err
	}
	defer container.Delete(ctx, containerd.WithSnapshotCleanup)

	// create a task from the container
	client.TaskService().Delete(ctx, &tasks.DeleteTaskRequest{
		ContainerID: container.ID(),
	})
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return fmt.Errorf("failed to create new task, %w", err)
	}
	defer task.Delete(ctx)
	// metrics, err := task.Metrics(ctx)

	labels := map[string]string{}
	_, err = cninetwork.CreateCNINetwork(ctx, cni, task, labels)

	if err != nil {
		return err
	}

	ip, err := cninetwork.GetIPAddress(containerName, task.Pid())
	if err != nil {
		return err
	}

	log.Printf("%s has IP: %s.\n", containerName, ip)

	// make sure we wait before calling start
	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		fmt.Println(err)
	}

	// call start on the task to execute the redis server
	if err := task.Start(ctx); err != nil {
		return fmt.Errorf("failed to start task, %w", err)
	}

	go func() {
		// sleep for a lil bit to see the logs
		time.Sleep(3 * time.Second)

		// kill the process and get the exit status
		if err := task.Kill(ctx, syscall.SIGTERM); err != nil {
			st, serr := task.Status(ctx)
			log.Printf("kill failed, %s, exit status=%s(%v), err %v", err.Error(), st.Status, st.ExitStatus, serr)
		}
	}()
	// wait for the process to fully exit and print out the exit status

	status := <-exitStatusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	fmt.Printf("redis-server exited with status: %d\n", code)

	return nil
}
