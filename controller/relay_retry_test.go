package controller

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/model"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestShouldRetryDoesNotRetrySpecificChannelForChannelError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("specific_channel_id", "1")
	err := types.NewError(errors.New("channel invalid key"), types.ErrorCodeChannelInvalidKey, types.ErrOptionWithStatusCode(http.StatusUnauthorized))

	require.False(t, shouldRetry(c, err, 1))
}

func TestRelayRetriesToNextChannelAfterChannelFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)

	firstUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"first channel rejected key","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	defer firstUpstream.Close()

	secondUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-retry-ok","object":"chat.completion","created":1,"model":"gpt-retry-relay","choices":[{"index":0,"message":{"role":"assistant","content":"fallback-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer secondUpstream.Close()

	channelOne := createRelayRetryFallbackChannel(t, 1, "first", firstUpstream.URL, 20)
	createRelayRetryFallbackChannel(t, 2, "second", secondUpstream.URL, 10)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	defer common.CleanupBodyStorage(c)
	setRelayRetryFallbackRequestContext(c)

	// 模拟 Distribute 中间件首次选中的高优先级渠道；Relay 内部重试必须自行排除失败渠道并重选。
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channelOne, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "fallback-ok")
	require.Equal(t, []string{"1", "2"}, c.GetStringSlice("use_channel"))
	require.Equal(t, []int{1}, service.GetExcludedChannelIds(c))

	var firstStored model.Channel
	require.NoError(t, model.DB.First(&firstStored, 1).Error)
	require.Equal(t, common.ChannelStatusEnabled, firstStored.Status)
}

func TestRelayRetriesToNextChannelAccountAfterSyncedKeyFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)

	firstUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer sk-account-fail", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"synced key rejected","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	defer firstUpstream.Close()

	secondUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer sk-account-ok", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-account-retry-ok","object":"chat.completion","created":1,"model":"gpt-retry-relay","choices":[{"index":0,"message":{"role":"assistant","content":"account-fallback-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer secondUpstream.Close()

	channel := createRelayRetryFallbackAccountPoolChannel(t, 1, "synced-account-channel")
	createRelayRetryFallbackChannelAccount(t, channel.Id, "first", "sk-account-fail", firstUpstream.URL, 20, 100)
	createRelayRetryFallbackChannelAccount(t, channel.Id, "second", "sk-account-ok", secondUpstream.URL, 10, 100)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	defer common.CleanupBodyStorage(c)
	setRelayRetryFallbackRequestContext(c)

	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "account-fallback-ok")
	require.Equal(t, []string{"1", "1"}, c.GetStringSlice("use_channel"))
	require.Empty(t, service.GetExcludedChannelIds(c))
	require.Len(t, service.GetExcludedChannelAccountIds(c), 1)

	var firstStored model.ChannelAccount
	require.NoError(t, model.DB.Where("channel_id = ? AND name = ?", channel.Id, "first").First(&firstStored).Error)
	require.Equal(t, common.ChannelStatusEnabled, firstStored.Status)
	require.Contains(t, firstStored.LastError, "synced key rejected")
	require.True(t, service.GetExcludedChannelAccountIds(c)[firstStored.Id])
}

func TestRelayAutomaticallyRetries503WithRetryTimesZeroToNextChannelAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)
	common.RetryTimes = 0

	var mu sync.Mutex
	seenKeys := make([]string, 0, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		auth := r.Header.Get("Authorization")
		mu.Lock()
		seenKeys = append(seenKeys, auth)
		mu.Unlock()
		if auth == "Bearer sk-account-fail" {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"Service temporarily unavailable","type":"server_error","code":"service_unavailable"}}`))
			return
		}
		require.Equal(t, "Bearer sk-account-ok", auth)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-service-unavailable-ok","object":"chat.completion","created":1,"model":"gpt-retry-relay","choices":[{"index":0,"message":{"role":"assistant","content":"account-fallback-ok-after-503"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	channel := createRelayRetryFallbackAccountPoolChannel(t, 1, "service-unavailable-account-channel")
	createRelayRetryFallbackChannelAccount(t, channel.Id, "first", "sk-account-fail", upstream.URL, 20, 100)
	createRelayRetryFallbackChannelAccount(t, channel.Id, "second", "sk-account-ok", upstream.URL, 10, 100)
	model.InitChannelCache()

	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c, recorder := newRelayRetryFallbackRequestContext(t, "/v1/chat/completions", body)
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "account-fallback-ok-after-503")
	require.Equal(t, []string{"1", "1"}, c.GetStringSlice("use_channel"))
	require.Empty(t, service.GetExcludedChannelIds(c))
	require.Len(t, service.GetExcludedChannelAccountIds(c), 1)
	require.Equal(t, []string{"Bearer sk-account-fail", "Bearer sk-account-ok"}, seenKeys)

	var firstStored model.ChannelAccount
	require.NoError(t, model.DB.Where("channel_id = ? AND name = ?", channel.Id, "first").First(&firstStored).Error)
	require.True(t, firstStored.OverloadUntil > common.GetTimestamp())
	require.Contains(t, firstStored.LastError, "Service temporarily unavailable")
	require.True(t, service.GetExcludedChannelAccountIds(c)[firstStored.Id])
}

func TestShouldRetryRelayStopsAfterDownstreamWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	_, _ = c.Writer.Write([]byte("already-sent"))
	relayInfo := &relaycommon.RelayInfo{
		StartTime:         time.Now().Add(-time.Second),
		FirstResponseTime: time.Now(),
	}
	err := types.NewOpenAIError(errors.New("Service temporarily unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)

	require.False(t, shouldRetryRelay(c, relayInfo, err, 0))
}

func TestRelayDoesNotAutomaticallyRetry503ForSpecificChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)
	common.RetryTimes = 0

	var hits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"Service temporarily unavailable","type":"server_error","code":"service_unavailable"}}`))
	}))
	defer upstream.Close()

	channel := createRelayRetryFallbackChannel(t, 1, "specific-service-unavailable-channel", upstream.URL, 20)
	createRelayRetryFallbackChannel(t, 2, "fallback", upstream.URL, 10)
	model.InitChannelCache()

	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c, recorder := newRelayRetryFallbackRequestContext(t, "/v1/chat/completions", body)
	c.Set("specific_channel_id", "1")
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Service temporarily unavailable")
	require.Equal(t, []string{"1"}, c.GetStringSlice("use_channel"))
	require.Equal(t, 1, hits)
}

func TestRelayAutomatic503RetriesStopAfterTwoExtraAttempts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)
	common.RetryTimes = 0

	fail := func(label string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/v1/chat/completions", r.URL.Path)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"Service temporarily unavailable ` + label + `","type":"server_error","code":"service_unavailable"}}`))
		}
	}
	firstUpstream := httptest.NewServer(fail("first"))
	defer firstUpstream.Close()
	secondUpstream := httptest.NewServer(fail("second"))
	defer secondUpstream.Close()
	thirdUpstream := httptest.NewServer(fail("third"))
	defer thirdUpstream.Close()

	first := createRelayRetryFallbackChannel(t, 1, "first-service-unavailable-channel", firstUpstream.URL, 30)
	createRelayRetryFallbackChannel(t, 2, "second-service-unavailable-channel", secondUpstream.URL, 20)
	createRelayRetryFallbackChannel(t, 3, "third-service-unavailable-channel", thirdUpstream.URL, 10)
	model.InitChannelCache()

	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c, recorder := newRelayRetryFallbackRequestContext(t, "/v1/chat/completions", body)
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, first, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Service temporarily unavailable third")
	require.Equal(t, []string{"1", "2", "3"}, c.GetStringSlice("use_channel"))
	require.Equal(t, []int{1, 2, 3}, service.GetExcludedChannelIds(c))
}

func TestRelayRetriesToNextMultiKeyCandidateBeforeChannelFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)
	common.RetryTimes = 1

	var mu sync.Mutex
	seenKeys := make([]string, 0, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		auth := r.Header.Get("Authorization")
		mu.Lock()
		seenKeys = append(seenKeys, auth)
		mu.Unlock()
		if auth == "Bearer sk-multi-fail" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"first multi key rejected","type":"invalid_request_error","code":"invalid_api_key"}}`))
			return
		}
		require.Equal(t, "Bearer sk-multi-ok", auth)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-multi-key-ok","object":"chat.completion","created":1,"model":"gpt-retry-relay","choices":[{"index":0,"message":{"role":"assistant","content":"multi-key-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	var fallbackHits int
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fallback.Close()

	channel := createRelayRetryFallbackMultiKeyChannel(t, 11, "multi-key-channel", upstream.URL, 20)
	createRelayRetryFallbackChannel(t, 12, "fallback", fallback.URL, 10)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	defer common.CleanupBodyStorage(c)
	setRelayRetryFallbackRequestContext(c)

	candidate, _, err := service.SelectRoutingCandidate(&service.RetryParam{
		Ctx:         c,
		TokenGroup:  "default",
		ModelName:   "gpt-retry-relay",
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.Equal(t, model.RoutingCredentialKindMultiKey, candidate.Kind)
	require.Equal(t, 0, candidate.MultiKeyIndex)
	require.Nil(t, middleware.SetupContextForRoutingCandidate(c, candidate, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "multi-key-ok")
	require.Equal(t, []string{"11", "11"}, c.GetStringSlice("use_channel"))
	require.Empty(t, service.GetExcludedChannelIds(c))
	require.Equal(t, 0, fallbackHits)
	require.Equal(t, []string{"Bearer sk-multi-fail", "Bearer sk-multi-ok"}, seenKeys)
	require.True(t, service.GetExcludedRoutingCandidateKeys(c)[(&model.RoutingCandidate{
		ChannelID:     channel.Id,
		Kind:          model.RoutingCredentialKindMultiKey,
		MultiKeyIndex: 0,
	}).CandidateKey().String()])
}

func TestRelayAffinitySkipRetryStillAllowsCredentialDegradation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)
	common.RetryTimes = 1

	var mu sync.Mutex
	seenKeys := make([]string, 0, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		auth := r.Header.Get("Authorization")
		mu.Lock()
		seenKeys = append(seenKeys, auth)
		mu.Unlock()
		if auth == "Bearer sk-multi-fail" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"affinity key rejected","type":"invalid_request_error","code":"invalid_api_key"}}`))
			return
		}
		require.Equal(t, "Bearer sk-multi-ok", auth)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-affinity-retry-ok","object":"chat.completion","created":1,"model":"gpt-retry-relay","choices":[{"index":0,"message":{"role":"assistant","content":"affinity-fallback-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	channel := createRelayRetryFallbackMultiKeyChannel(t, 41, "affinity-multi-key-channel", upstream.URL, 20)
	model.InitChannelCache()
	seedRelayRetryAffinity(t, "affinity-skip-key", channel.Id)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Test-Affinity", "affinity-skip-key")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	defer common.CleanupBodyStorage(c)
	setRelayRetryFallbackRequestContext(c)

	preferredID, found := service.GetPreferredChannelByAffinity(c, "gpt-retry-relay", "default")
	require.True(t, found)
	require.Equal(t, channel.Id, preferredID)
	service.MarkChannelAffinityUsed(c, "default", channel.Id)
	require.True(t, service.ShouldSkipRetryAfterChannelAffinityFailure(c))
	candidate, _, err := service.SelectRoutingCandidate(&service.RetryParam{
		Ctx:         c,
		TokenGroup:  "default",
		ModelName:   "gpt-retry-relay",
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.Nil(t, middleware.SetupContextForRoutingCandidate(c, candidate, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "affinity-fallback-ok")
	require.Equal(t, []string{"41", "41"}, c.GetStringSlice("use_channel"))
	require.Equal(t, []string{"Bearer sk-multi-fail", "Bearer sk-multi-ok"}, seenKeys)
	require.False(t, service.ShouldSkipRetryAfterChannelAffinityFailure(c))
}

func TestRelayFallsBackToNextChannelAfterAllMultiKeyCandidatesFail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)
	common.RetryTimes = 2

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"multi key rejected","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	defer upstream.Close()

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-after-multi-key-ok","object":"chat.completion","created":1,"model":"gpt-retry-relay","choices":[{"index":0,"message":{"role":"assistant","content":"channel-fallback-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer fallback.Close()

	channel := createRelayRetryFallbackMultiKeyChannel(t, 21, "multi-key-channel", upstream.URL, 20)
	createRelayRetryFallbackChannel(t, 22, "fallback", fallback.URL, 10)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	defer common.CleanupBodyStorage(c)
	setRelayRetryFallbackRequestContext(c)

	candidate, _, err := service.SelectRoutingCandidate(&service.RetryParam{
		Ctx:         c,
		TokenGroup:  "default",
		ModelName:   "gpt-retry-relay",
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.Equal(t, model.RoutingCredentialKindMultiKey, candidate.Kind)
	require.Equal(t, 0, candidate.MultiKeyIndex)
	require.Nil(t, middleware.SetupContextForRoutingCandidate(c, candidate, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "channel-fallback-ok")
	require.Equal(t, []string{"21", "21", "22"}, c.GetStringSlice("use_channel"))
	require.Equal(t, []int{channel.Id}, service.GetExcludedChannelIds(c))
	require.Len(t, service.GetExcludedRoutingCandidateKeys(c), 2)
}

func TestRelayDoesNotRetryMultiKeyWhenRetryTimesIsZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)
	common.RetryTimes = 0

	var mu sync.Mutex
	seenKeys := make([]string, 0, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		mu.Lock()
		seenKeys = append(seenKeys, auth)
		mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"first multi key rejected","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	defer upstream.Close()

	candidateChannel := createRelayRetryFallbackMultiKeyChannel(t, 31, "multi-key-channel", upstream.URL, 20)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	defer common.CleanupBodyStorage(c)
	setRelayRetryFallbackRequestContext(c)

	candidate, _, err := service.SelectRoutingCandidate(&service.RetryParam{
		Ctx:         c,
		TokenGroup:  "default",
		ModelName:   "gpt-retry-relay",
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.Equal(t, model.RoutingCredentialKindMultiKey, candidate.Kind)
	require.Nil(t, middleware.SetupContextForRoutingCandidate(c, candidate, "gpt-retry-relay"))

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Equal(t, []string{"31"}, c.GetStringSlice("use_channel"))
	require.Empty(t, service.GetExcludedChannelIds(c))
	require.Equal(t, []string{"Bearer sk-multi-fail"}, seenKeys)
	require.True(t, service.GetExcludedRoutingCandidateKeys(c)[(&model.RoutingCandidate{
		ChannelID:     candidateChannel.Id,
		Kind:          model.RoutingCredentialKindMultiKey,
		MultiKeyIndex: 0,
	}).CandidateKey().String()])
}

func TestPrepareRelayChannelContextFallsBackWhenInitialAccountPoolUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRelayRetryFallbackTestState(t)

	accountPoolChannel := createRelayRetryFallbackAccountPoolChannel(t, 1, "cooling-account-channel")
	createRelayRetryFallbackCoolingChannelAccount(t, accountPoolChannel.Id, "cooling", "sk-cooling", 20, 100)
	fallbackChannel := createRelayRetryFallbackChannel(t, 2, "fallback", "https://fallback.example.test", 10)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-retry-relay","messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	defer common.CleanupBodyStorage(c)
	setRelayRetryFallbackRequestContext(c)

	selected, ok := middleware.PrepareRelayChannelContext(c)

	require.True(t, ok)
	require.NotNil(t, selected)
	require.Equal(t, fallbackChannel.Id, selected.Id)
	require.Empty(t, service.GetExcludedChannelIds(c))
	require.Len(t, service.GetExcludedRoutingCandidateKeys(c), 1)
	require.Equal(t, fallbackChannel.Id, common.GetContextKeyInt(c, constant.ContextKeyChannelId))
}

func setupRelayRetryFallbackTestState(t *testing.T) {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	oldLogConsumeEnabled := common.LogConsumeEnabled
	oldCountToken := constant.CountToken
	oldRetryTimes := common.RetryTimes
	oldQuotaSetting := *operation_setting.GetQuotaSetting()
	oldModelRatio := ratio_setting.GetModelRatioCopy()
	oldGroupRatio := ratio_setting.GetGroupRatioCopy()
	oldModelRatioJSON, err := common.Marshal(oldModelRatio)
	require.NoError(t, err)
	oldGroupRatioJSON, err := common.Marshal(oldGroupRatio)
	require.NoError(t, err)

	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = false
	constant.CountToken = false
	common.RetryTimes = 2
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-retry-relay":0}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	service.InitHttpClient()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}, &model.Token{}, &model.Log{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.Create(&model.User{
		Id:       1,
		Username: "relay-retry-user",
		Password: "not-used-in-test",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    1_000_000,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:             1,
		UserId:         1,
		Key:            "relay-retry-token",
		Status:         common.TokenStatusEnabled,
		Name:           "relay-retry-token",
		RemainQuota:    1_000_000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
		common.LogConsumeEnabled = oldLogConsumeEnabled
		constant.CountToken = oldCountToken
		common.RetryTimes = oldRetryTimes
		*operation_setting.GetQuotaSetting() = oldQuotaSetting
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(oldModelRatioJSON)))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(oldGroupRatioJSON)))
	})
}

func newRelayRetryFallbackRequestContext(t *testing.T, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	setRelayRetryFallbackRequestContext(c)
	return c, recorder
}

func seedRelayRetryAffinity(t *testing.T, headerValue string, channelID int) {
	t.Helper()
	setting := operation_setting.GetChannelAffinitySetting()
	original := *setting
	originalRules := append([]operation_setting.ChannelAffinityRule(nil), setting.Rules...)
	setting.Enabled = true
	setting.SwitchOnSuccess = true
	setting.KeepOnChannelDisabled = false
	setting.DefaultTTLSeconds = 3600
	setting.Rules = []operation_setting.ChannelAffinityRule{
		{
			Name:       "relay retry affinity",
			ModelRegex: []string{"^gpt-retry-relay$"},
			PathRegex:  []string{"/v1/chat/completions"},
			KeySources: []operation_setting.ChannelAffinityKeySource{
				{Type: "request_header", Key: "X-Test-Affinity"},
			},
			SkipRetryOnFailure: true,
			IncludeUsingGroup:  true,
			IncludeRuleName:    true,
		},
	}

	seedCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	seedCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	seedCtx.Request.Header.Set("X-Test-Affinity", headerValue)
	_, _ = service.GetPreferredChannelByAffinity(seedCtx, "gpt-retry-relay", "default")
	service.RecordChannelAffinity(seedCtx, channelID)

	t.Cleanup(func() {
		service.ClearCurrentChannelAffinityCache(seedCtx)
		*setting = original
		setting.Rules = originalRules
	})
}

func createRelayRetryFallbackChannel(t *testing.T, id int, name string, baseURL string, priority int64) *model.Channel {
	t.Helper()

	weight := uint(100)
	autoBan := 0
	channel := &model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-test",
		Status:   common.ChannelStatusEnabled,
		Name:     name,
		Weight:   &weight,
		BaseURL:  &baseURL,
		Models:   "gpt-retry-relay",
		Group:    "default",
		Priority: &priority,
		AutoBan:  &autoBan,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-retry-relay",
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	return channel
}

func createRelayRetryFallbackMultiKeyChannel(t *testing.T, id int, name string, baseURL string, priority int64) *model.Channel {
	t.Helper()

	weight := uint(100)
	autoBan := 0
	channel := &model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-multi-fail\nsk-multi-ok",
		Status:   common.ChannelStatusEnabled,
		Name:     name,
		Weight:   &weight,
		BaseURL:  &baseURL,
		Models:   "gpt-retry-relay",
		Group:    "default",
		Priority: &priority,
		AutoBan:  &autoBan,
		ChannelInfo: model.ChannelInfo{
			CredentialMode: constant.ChannelCredentialModeMultiKey,
			IsMultiKey:     true,
			MultiKeySize:   2,
			MultiKeyMode:   constant.MultiKeyModePolling,
		},
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-retry-relay",
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	return channel
}

func createRelayRetryFallbackAccountPoolChannel(t *testing.T, id int, name string) *model.Channel {
	t.Helper()

	weight := uint(100)
	priority := int64(20)
	autoBan := 0
	channel := &model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      constant.ChannelCredentialModeAccountPool,
		Status:   common.ChannelStatusEnabled,
		Name:     name,
		Weight:   &weight,
		Models:   "gpt-retry-relay",
		Group:    "default",
		Priority: &priority,
		AutoBan:  &autoBan,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-retry-relay",
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	return channel
}

func createRelayRetryFallbackChannelAccount(t *testing.T, channelID int, name string, key string, baseURL string, priority int64, weight int) {
	t.Helper()

	require.NoError(t, model.DB.Create(&model.ChannelAccount{
		ChannelId: channelID,
		Name:      name,
		Key:       key,
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-retry-relay",
		Group:     "default",
		BaseURL:   &baseURL,
		Priority:  priority,
		Weight:    weight,
	}).Error)
}

func createRelayRetryFallbackCoolingChannelAccount(t *testing.T, channelID int, name string, key string, priority int64, weight int) {
	t.Helper()

	require.NoError(t, model.DB.Create(&model.ChannelAccount{
		ChannelId:        channelID,
		Name:             name,
		Key:              key,
		Status:           common.ChannelStatusEnabled,
		Models:           "gpt-retry-relay",
		Group:            "default",
		Priority:         priority,
		Weight:           weight,
		RateLimitedUntil: common.GetTimestamp() + 300,
	}).Error)
}

func setRelayRetryFallbackRequestContext(c *gin.Context) {
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserQuota, 1_000_000)
	common.SetContextKey(c, constant.ContextKeyUserEmail, "relay-retry@example.test")
	common.SetContextKey(c, constant.ContextKeyTokenId, 1)
	common.SetContextKey(c, constant.ContextKeyTokenKey, "relay-retry-token")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, false)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-retry-relay")
	common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{})
	c.Set("token_name", "relay-retry-token")
	c.Set("username", "relay-retry-user")
}
