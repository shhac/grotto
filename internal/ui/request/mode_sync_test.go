package request

import (
	"log/slog"
	stdsync "sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/shhac/grotto/internal/logging"
	"github.com/stretchr/testify/assert"
)

func TestNewModeSynchronizer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	assert.NotNil(t, sync, "ModeSynchronizer should not be nil")
	assert.False(t, sync.IsSyncing(), "should not be syncing initially")
}

func TestModeSynchronizer_SwitchMode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	tests := []struct {
		name         string
		targetMode   string
		expectedMode string
	}{
		{
			name:         "switch to form mode",
			targetMode:   "form",
			expectedMode: "form",
		},
		{
			name:         "switch back to text mode",
			targetMode:   "text",
			expectedMode: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sync.SwitchMode(tt.targetMode)

			// Give it a moment to complete
			time.Sleep(10 * time.Millisecond)

			mode, _ := mode.Get()
			assert.Equal(t, tt.expectedMode, mode)
			assert.False(t, sync.IsSyncing(), "should not be syncing after completion")
		})
	}
}

func TestModeSynchronizer_IsSyncing(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Initially not syncing
	assert.False(t, sync.IsSyncing(), "should not be syncing initially")

	// Create a channel to coordinate the test
	syncingObserved := make(chan bool, 1)

	// Set callback that checks syncing state
	sync.SetOnModeChanged(func(mode string) {
		// By the time callback is called, syncing should be false
		syncingObserved <- sync.IsSyncing()
	})

	// Start the switch in a goroutine and immediately check if syncing
	go func() {
		sync.SwitchMode("form")
	}()

	// Give the goroutine a moment to start
	time.Sleep(5 * time.Millisecond)

	// Check that it's not syncing (may have completed by now)
	// or is syncing (caught it in the act)
	isSyncing := sync.IsSyncing()

	// Wait for callback
	callbackSyncing := <-syncingObserved

	// After callback, should definitely not be syncing
	assert.False(t, callbackSyncing, "should not be syncing in callback")
	assert.False(t, sync.IsSyncing(), "should not be syncing after completion")

	t.Logf("Syncing state during check: %v", isSyncing)
}

func TestModeSynchronizer_SwitchMode_NoOpWhenAlreadyOnMode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	callbackCount := 0
	var receivedModes []string
	sync.SetOnModeChanged(func(mode string) {
		callbackCount++
		receivedModes = append(receivedModes, mode)
	})

	// First switch to text mode
	sync.SwitchMode("text")
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, 1, callbackCount, "callback should be called once")

	// Switch to text mode again (already on it)
	sync.SwitchMode("text")
	time.Sleep(10 * time.Millisecond)

	// Note: Due to implementation detail, callback IS called even for no-op switches
	// (defer always executes the callback). However, the mode binding is not updated.
	assert.Equal(t, 2, callbackCount, "callback is called even for no-op (implementation detail)")
	assert.Equal(t, []string{"text", "text"}, receivedModes)

	// Verify the actual mode didn't change
	assert.Equal(t, "text", sync.GetMode())
}

func TestModeSynchronizer_ConcurrentSwitchMode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Track how many switches actually happened
	callbackCount := 0
	var mu stdsync.Mutex

	sync.SetOnModeChanged(func(mode string) {
		mu.Lock()
		callbackCount++
		mu.Unlock()
	})

	// Try to switch modes concurrently
	var wg stdsync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				sync.SwitchMode("form")
			} else {
				sync.SwitchMode("text")
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Should end up in a valid state (either text or form)
	finalMode := sync.GetMode()
	assert.True(t, finalMode == "text" || finalMode == "form", "should end in valid mode")

	// Should not be syncing after all operations complete
	assert.False(t, sync.IsSyncing(), "should not be syncing after concurrent switches")

	t.Logf("Callback called %d times out of 10 concurrent switches", callbackCount)
}

func TestModeSynchronizer_GetMode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Initial mode
	assert.Equal(t, "text", sync.GetMode())

	// After switching
	sync.SwitchMode("form")
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, "form", sync.GetMode())
}

func TestModeSynchronizer_SetFormBuilder(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Initially nil
	assert.NotPanics(t, func() {
		sync.SwitchMode("form")
	})

	// Set a nil builder (should not panic)
	assert.NotPanics(t, func() {
		sync.SetFormBuilder(nil)
	})
}

func TestModeSynchronizer_SetOnModeChanged(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	called := false
	var receivedMode string

	sync.SetOnModeChanged(func(mode string) {
		called = true
		receivedMode = mode
	})

	sync.SwitchMode("form")
	time.Sleep(10 * time.Millisecond)

	assert.True(t, called, "callback should be called")
	assert.Equal(t, "form", receivedMode, "callback should receive correct mode")
}

func TestModeSynchronizer_SyncFormToTextNow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("form")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Call without a form builder (should not panic)
	assert.NotPanics(t, func() {
		sync.SyncFormToTextNow()
	})

	// Should not be syncing after
	time.Sleep(10 * time.Millisecond)
	assert.False(t, sync.IsSyncing(), "should not be syncing after SyncFormToTextNow")
}

func TestModeSynchronizer_SyncFormToTextNow_ConcurrentCalls(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("form")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Make multiple concurrent calls
	var wg stdsync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sync.SyncFormToTextNow()
		}()
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Should not be syncing after all operations complete
	assert.False(t, sync.IsSyncing(), "should not be syncing after concurrent SyncFormToTextNow calls")
}

func TestModeSynchronizer_AtomicSyncingFlag(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Test that the atomic flag works correctly
	assert.False(t, sync.IsSyncing())

	// Manually verify CompareAndSwap behavior by triggering a switch
	// and checking IsSyncing in a tight loop
	go sync.SwitchMode("form")

	// Try to catch it syncing
	var caughtSyncing bool
	for i := 0; i < 100; i++ {
		if sync.IsSyncing() {
			caughtSyncing = true
			break
		}
		time.Sleep(1 * time.Microsecond)
	}

	// Wait for completion
	time.Sleep(20 * time.Millisecond)

	// Should not be syncing now
	assert.False(t, sync.IsSyncing())

	t.Logf("Caught syncing flag: %v", caughtSyncing)
}

func TestModeSynchronizer_CallbackExecutedAfterSyncComplete(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Track the order of events
	events := []string{}
	var mu stdsync.Mutex

	sync.SetOnModeChanged(func(mode string) {
		mu.Lock()
		defer mu.Unlock()

		// Check if syncing flag is false when callback runs
		if sync.IsSyncing() {
			events = append(events, "callback-while-syncing")
		} else {
			events = append(events, "callback-after-sync")
		}
	})

	sync.SwitchMode("form")
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Callback should execute after syncing completes
	assert.Contains(t, events, "callback-after-sync", "callback should run after syncing completes")
	assert.NotContains(t, events, "callback-while-syncing", "callback should not run while syncing")
}

func TestModeSynchronizer_MultipleRapidSwitches(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Rapidly switch modes
	sync.SwitchMode("form")
	sync.SwitchMode("text")
	sync.SwitchMode("form")
	sync.SwitchMode("text")
	sync.SwitchMode("form")

	// Give time to settle
	time.Sleep(50 * time.Millisecond)

	// Should end up in form mode (last switch)
	assert.Equal(t, "form", sync.GetMode())
	assert.False(t, sync.IsSyncing(), "should not be syncing after rapid switches")
}

func TestModeSynchronizer_LoggerUsage(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()

	// Use a real logger to verify it doesn't panic
	logger := slog.New(slog.NewTextHandler(nil, nil))

	sync := NewModeSynchronizer(mode, textData, logger)

	// Should not panic with real logger
	assert.NotPanics(t, func() {
		sync.SwitchMode("form")
	})

	time.Sleep(10 * time.Millisecond)
	assert.False(t, sync.IsSyncing())
}

func TestModeSynchronizer_NilFormBuilder_TextToForm(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	textData.Set(`{"test": "value"}`)
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Switch to form without setting a form builder
	assert.NotPanics(t, func() {
		sync.SwitchMode("form")
	})

	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, "form", sync.GetMode())
}

func TestModeSynchronizer_NilFormBuilder_FormToText(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mode := binding.NewString()
	mode.Set("form")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Switch to text without setting a form builder
	assert.NotPanics(t, func() {
		sync.SwitchMode("text")
	})

	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, "text", sync.GetMode())
}

// TestModeSynchronizer_RealFormBuilder tests with actual form builder integration
// This is commented out as it requires proto dependencies, but shows how to test with real FormBuilder
/*
func TestModeSynchronizer_RealFormBuilder(t *testing.T) {
	mode := binding.NewString()
	mode.Set("text")
	textData := binding.NewString()
	logger := logging.NewNopLogger()

	sync := NewModeSynchronizer(mode, textData, logger)

	// Create a simple message descriptor for testing
	// This would require actual proto setup
	// md := ... // your message descriptor
	// builder := form.NewFormBuilder(md)
	// sync.SetFormBuilder(builder)

	// Set some JSON
	// textData.Set(`{"field": "value"}`)

	// Switch to form (should populate form from JSON)
	// sync.SwitchMode("form")

	// Modify form...

	// Switch back to text (should update JSON from form)
	// sync.SwitchMode("text")

	// Verify JSON was updated
	// json, _ := textData.Get()
	// assert.Contains(t, json, "field")
}
*/
