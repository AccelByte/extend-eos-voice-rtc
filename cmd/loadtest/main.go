package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"extend-eos-voice-rtc/pkg/service"
	"extend-eos-voice-rtc/pkg/voiceclient"

	lobbyNotification "github.com/AccelByte/accelbyte-go-sdk/lobby-sdk/pkg/lobbyclient/notification"
	"github.com/sirupsen/logrus"
)

func main() {
	var (
		requests   = flag.Int("requests", 1000, "Number of game-session-created events to simulate")
		workers    = flag.Int("workers", runtime.NumCPU(), "Number of concurrent workers")
		players    = flag.Int("participants", 8, "Participants per simulated snapshot")
		cpuprofile = flag.String("cpuprofile", "", "Write CPU profile to the provided path")
		memprofile = flag.String("memprofile", "", "Write heap profile to the provided path after the run")
	)
	flag.Parse()

	if *requests <= 0 {
		fmt.Fprintln(os.Stderr, "requests must be greater than 0")
		os.Exit(1)
	}
	if *workers <= 0 {
		*workers = 1
	}
	if *players <= 0 {
		*players = 1
	}

	logger := logrus.New().WithField("component", "loadtest")
	processor, err := service.NewVoiceEventProcessor(service.VoiceProcessorConfig{
		Namespace:           "loadtest",
		NotificationTopic:   "LOADTEST",
		VoiceClient:         &loadTestVoiceClient{},
		NotificationService: &loadTestNotifier{},
		Logger:              logger,
		EnableTeamVoice:     true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to construct voice processor: %v\n", err)
		os.Exit(1)
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create CPU profile: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "unable to start CPU profile: %v\n", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}

	ctx := context.Background()
	var success int64
	var failures int64

	start := time.Now()
	jobs := make(chan int, *workers)
	wg := sync.WaitGroup{}

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				sessionID := fmt.Sprintf("session-%d", job)
				payload := makeSnapshot(sessionID, *players)
				if err := processor.HandleGameSessionCreated(ctx, sessionID, payload); err != nil {
					atomic.AddInt64(&failures, 1)
					continue
				}
				atomic.AddInt64(&success, 1)
			}
		}()
	}

	for i := 0; i < *requests; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	duration := time.Since(start)
	rate := float64(*requests) / duration.Seconds()
	fmt.Printf("Processed %d events (%d failures) in %s (%.2f req/s)\n", success, failures, duration.Truncate(time.Millisecond), rate)

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create heap profile: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write heap profile: %v\n", err)
			os.Exit(1)
		}
	}
}

func makeSnapshot(sessionID string, participants int) string {
	payload := map[string]any{
		"payload": map[string]any{
			"ID":      sessionID,
			"Members": make([]map[string]string, 0, participants),
			"Teams":   []map[string]any{},
		},
	}
	members := payload["payload"].(map[string]any)["Members"].([]map[string]string)
	for i := 0; i < participants; i++ {
		members = append(members, map[string]string{
			"ID":       fmt.Sprintf("%s-user-%d", sessionID, i),
			"Status":   "JOINED",
			"StatusV2": "JOINED",
		})
	}
	payload["payload"].(map[string]any)["Members"] = members
	data, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(data)
}

type loadTestVoiceClient struct{}

func (l *loadTestVoiceClient) CreateRoomTokens(_ context.Context, roomID string, participants []voiceclient.Participant) (*voiceclient.CreateRoomTokenResponse, error) {
	resp := &voiceclient.CreateRoomTokenResponse{
		RoomID:        roomID,
		ClientBaseURL: "wss://example",
		Participants:  make([]voiceclient.CreateRoomTokenParticipant, 0, len(participants)),
	}
	for _, p := range participants {
		resp.Participants = append(resp.Participants, voiceclient.CreateRoomTokenParticipant{
			ProductUserID: p.ProductUserID,
			Token:         fmt.Sprintf("token-%s", p.ProductUserID),
		})
	}
	return resp, nil
}

func (l *loadTestVoiceClient) RemoveParticipant(context.Context, string, string) error {
	return nil
}

func (l *loadTestVoiceClient) QueryExternalAccounts(_ context.Context, _ string, accountIDs []string) (map[string]string, error) {
	result := make(map[string]string, len(accountIDs))
	for _, id := range accountIDs {
		result[id] = fmt.Sprintf("puid-%s", id)
	}
	return result, nil
}

type loadTestNotifier struct{}

func (l *loadTestNotifier) SendSpecificUserFreeformNotificationV1AdminShort(*lobbyNotification.SendSpecificUserFreeformNotificationV1AdminParams) error {
	return nil
}
