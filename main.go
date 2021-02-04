package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/api"
)

var (
	help = flag.Bool("help", false, "Print help message")
	url  = flag.String("url", "", "The Nomad API address")
	job  = flag.String("job", "*", "The job to catch the eventstream for (defaults to all jobs)")
)

type Stream struct {
	nomad *api.Client
	L     hclog.Logger
}

func main() {
	flag.Parse()

	if *help == true {
		flag.Usage()
		os.Exit(0)
	}

	ctx, closer := CtxWithInterrupt(context.Background())
	defer closer()

	stream := NewStream()

	events := stream.nomad.EventStream()

	topics := map[api.Topic][]string{
		api.Topic("Allocation"): {*job},
	}
	eventCh, err := events.Stream(ctx, topics, 0, &api.QueryOptions{})
	if err != nil {
		stream.L.Error("Error creating event stream client", "error", err)
		os.Exit(1)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-eventCh:
			if event.Err != nil {
				stream.L.Warn("Error from event stream", "error", err)
				break
			}

			if event.IsHeartbeat() {
				continue
			}

			for _, e := range event.Events {
				alloc, err := e.Allocation()
				if err != nil {
					stream.L.Error("Expected allocation", "error", err)
					continue
				}

				alloc_json, err := json.Marshal(alloc)
				var allocation api.Allocation
				err = json.Unmarshal(alloc_json, &allocation)

				if err != nil {
					stream.L.Error("Error parsing JSON", "error", err)
				}

				taskstate_json, err := json.Marshal(allocation.TaskStates)
				var taskstates api.TaskState
				err = json.Unmarshal(taskstate_json, &taskstates)

				if err != nil {
					stream.L.Error("Error parsing JSON", "error", err)
				}

				events_json, err := json.Marshal(taskstates.Events)
				var events api.Events
				err = json.Unmarshal(events_json, &events)

				if err != nil {
					stream.L.Error("Error parsing JSON", "error", err)
				}

				for taskName, _ := range allocation.TaskStates {
					events_length := len(allocation.TaskStates[taskName].Events)
					timestamp := time.Unix(0, allocation.TaskStates[taskName].Events[events_length-1].Time)
					fmt.Printf("timestamp=%s, node=%s, alloc_id=%s, job_id=%s, task_id=%s namespace=%s, job_status=%s, task_state=%s, event_type=%s, event=%s \n",
						timestamp,
						allocation.NodeName,
						allocation.ID,
						allocation.JobID,
						taskName,
						allocation.Namespace,
						allocation.ClientStatus,
						allocation.TaskStates[taskName].State,
						allocation.TaskStates[taskName].Events[events_length-1].Type,
						allocation.TaskStates[taskName].Events[events_length-1].DisplayMessage,
					)
				}
			}
		}
	}
}

func NewStream() *Stream {
	client, _ := api.NewClient(&api.Config{
		Address: *url,
		TLSConfig: &api.TLSConfig{
			Insecure: true,
		},
	})

	return &Stream{
		nomad: client,
		L:     hclog.Default(),
	}
}

func CtxWithInterrupt(ctx context.Context) (context.Context, func()) {

	ctx, cancel := context.WithCancel(ctx)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	return ctx, func() {
		signal.Stop(ch)
		cancel()
	}
}
