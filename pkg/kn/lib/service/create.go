package service

import (
	"fmt"
	"io"
	"time"

	"knative.dev/client/pkg/kn/commands"
	clientservingv1 "knative.dev/client/pkg/serving/v1"
	"knative.dev/client/pkg/wait"
	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// CreateService creates a service
func CreateService(client clientservingv1.KnServingClient, service *servingv1.Service, waitFlags commands.WaitFlags, out io.Writer) error {
	err := client.CreateService(service)
	if err != nil {
		return err
	}

	return waitIfRequested(client, service, waitFlags, "Creating", "created", out)
}

//ReplaceService replaces an existing service
func ReplaceService(client clientservingv1.KnServingClient, service *servingv1.Service, waitFlags commands.WaitFlags, out io.Writer) error {
	err := prepareAndUpdateService(client, service)
	if err != nil {
		return err
	}
	return waitIfRequested(client, service, waitFlags, "Replacing", "replaced", out)
}

func waitIfRequested(client clientservingv1.KnServingClient, service *servingv1.Service, waitFlags commands.WaitFlags, verbDoing string, verbDone string, out io.Writer) error {
	//TODO: deprecated condition should be removed with --async flag
	if waitFlags.Async {
		fmt.Fprintf(out, "\nWARNING: flag --async is deprecated and going to be removed in future release, please use --no-wait instead.\n\n")
		fmt.Fprintf(out, "Service '%s' %s in namespace '%s'.\n", service.Name, verbDone, client.Namespace())
		return nil
	}
	if waitFlags.NoWait {
		fmt.Fprintf(out, "Service '%s' %s in namespace '%s'.\n", service.Name, verbDone, client.Namespace())
		return nil
	}

	fmt.Fprintf(out, "%s service '%s' in namespace '%s':\n", verbDoing, service.Name, client.Namespace())
	return waitForServiceToGetReady(client, service.Name, waitFlags.TimeoutInSeconds, verbDone, out)
}

func prepareAndUpdateService(client clientservingv1.KnServingClient, service *servingv1.Service) error {
	var retries = 0
	for {
		existingService, err := client.GetService(service.Name)
		if err != nil {
			return err
		}

		// Copy over some annotations that we want to keep around. Erase others
		copyList := []string{
			serving.CreatorAnnotation,
			serving.UpdaterAnnotation,
		}

		// If the target Annotation doesn't exist, create it even if
		// we don't end up copying anything over so that we erase all
		// existing annotations
		if service.Annotations == nil {
			service.Annotations = map[string]string{}
		}

		// Do the actual copy now, but only if it's in the source annotation
		for _, k := range copyList {
			if v, ok := existingService.Annotations[k]; ok {
				service.Annotations[k] = v
			}
		}

		service.ResourceVersion = existingService.ResourceVersion
		err = client.UpdateService(service)
		if err != nil {
			// Retry to update when a resource version conflict exists
			if apierrors.IsConflict(err) && retries < /* TODO MaxUpdateRetries */ 3 {
				retries++
				continue
			}
			return err
		}
		return nil
	}
}

func waitForServiceToGetReady(client clientservingv1.KnServingClient, name string, timeout int, verbDone string, out io.Writer) error {
	fmt.Fprintln(out, "")
	err := waitForService(client, name, out, timeout)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "")
	return showUrl(client, name, "", verbDone, out)
}

// ServiceExists returns true if the service exists
func ServiceExists(client clientservingv1.KnServingClient, name string) (bool, error) {
	_, err := client.GetService(name)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func waitForService(client clientservingv1.KnServingClient, serviceName string, out io.Writer, timeout int) error {
	err, duration := client.WaitForService(serviceName, time.Duration(timeout)*time.Second, wait.SimpleMessageCallback(out))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%7.3fs Ready to serve.\n", float64(duration.Round(time.Millisecond))/float64(time.Second))
	return nil
}

func showUrl(client clientservingv1.KnServingClient, serviceName string, originalRevision string, what string, out io.Writer) error {
	service, err := client.GetService(serviceName)
	if err != nil {
		return fmt.Errorf("cannot fetch service '%s' in namespace '%s' for extracting the URL: %v", serviceName, client.Namespace(), err)
	}

	url := service.Status.URL.String()

	newRevision := service.Status.LatestReadyRevisionName
	if originalRevision != "" && originalRevision == newRevision {
		fmt.Fprintf(out, "Service '%s' with latest revision '%s' (unchanged) is available at URL:\n%s\n", serviceName, newRevision, url)
	} else {
		fmt.Fprintf(out, "Service '%s' %s to latest revision '%s' is available at URL:\n%s\n", serviceName, what, newRevision, url)
	}

	return nil
}
