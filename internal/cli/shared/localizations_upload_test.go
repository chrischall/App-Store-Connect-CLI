package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
)

type stubVersionLocalizationClient struct {
	getResp  *asc.AppStoreVersionLocalizationsResponse
	getResps []*asc.AppStoreVersionLocalizationsResponse
	getErr   error
	getCalls int

	createErrs  []error
	createCalls []asc.AppStoreVersionLocalizationAttributes
	updateErrs  []error
	updateCalls []asc.AppStoreVersionLocalizationAttributes
}

type expiringVersionLocalizationClient struct {
	getCalls    int
	updateCalls int
}

func (c *expiringVersionLocalizationClient) GetAppStoreVersionLocalizations(ctx context.Context, _ string, _ ...asc.AppStoreVersionLocalizationsOption) (*asc.AppStoreVersionLocalizationsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.getCalls++
	description := "Old description"
	if c.getCalls > 1 {
		description = "New description"
	}
	return &asc.AppStoreVersionLocalizationsResponse{Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
		{ID: "existing-loc", Attributes: asc.AppStoreVersionLocalizationAttributes{Locale: "en-US", Description: description}},
	}}, nil
}

func (c *expiringVersionLocalizationClient) CreateAppStoreVersionLocalization(context.Context, string, asc.AppStoreVersionLocalizationAttributes) (*asc.AppStoreVersionLocalizationResponse, error) {
	return nil, errors.New("unexpected create")
}

func (c *expiringVersionLocalizationClient) UpdateAppStoreVersionLocalization(ctx context.Context, _ string, _ asc.AppStoreVersionLocalizationAttributes) (*asc.AppStoreVersionLocalizationResponse, error) {
	c.updateCalls++
	<-ctx.Done()
	return nil, &asc.RetryableError{Err: ctx.Err()}
}

func (s *stubVersionLocalizationClient) GetAppStoreVersionLocalizations(_ context.Context, _ string, _ ...asc.AppStoreVersionLocalizationsOption) (*asc.AppStoreVersionLocalizationsResponse, error) {
	s.getCalls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	if index := s.getCalls - 1; index >= 0 && index < len(s.getResps) {
		return s.getResps[index], nil
	}
	return s.getResp, nil
}

func (s *stubVersionLocalizationClient) CreateAppStoreVersionLocalization(_ context.Context, _ string, attrs asc.AppStoreVersionLocalizationAttributes) (*asc.AppStoreVersionLocalizationResponse, error) {
	s.createCalls = append(s.createCalls, attrs)
	callIndex := len(s.createCalls) - 1
	if callIndex < len(s.createErrs) && s.createErrs[callIndex] != nil {
		return nil, s.createErrs[callIndex]
	}
	return &asc.AppStoreVersionLocalizationResponse{
		Data: asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
			ID: "created-loc",
		},
	}, nil
}

func TestUploadVersionLocalizations_SkipsExactExistingValues(t *testing.T) {
	client := &stubVersionLocalizationClient{
		getResp: &asc.AppStoreVersionLocalizationsResponse{Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
			{
				ID: "existing-loc",
				Attributes: asc.AppStoreVersionLocalizationAttributes{
					Locale:      "en-US",
					Description: "Existing description",
					Keywords:    "one,two",
				},
			},
		}},
	}

	results, err := UploadVersionLocalizations(context.Background(), client, "version-id", map[string]map[string]string{
		"en-US": {
			"description": "Existing description",
			"keywords":    "one,two",
		},
	}, false)
	if err != nil {
		t.Fatalf("UploadVersionLocalizations() error: %v", err)
	}
	if len(client.updateCalls) != 0 || len(client.createCalls) != 0 {
		t.Fatalf("expected exact state to skip mutation, creates=%d updates=%d", len(client.createCalls), len(client.updateCalls))
	}
	if len(results) != 1 || results[0].Action != "skip" || results[0].LocalizationID != "existing-loc" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestUploadVersionLocalizations_ReconcilesAmbiguousUpdateWithoutReplay(t *testing.T) {
	client := &stubVersionLocalizationClient{
		getResps: []*asc.AppStoreVersionLocalizationsResponse{
			{Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
				{ID: "existing-loc", Attributes: asc.AppStoreVersionLocalizationAttributes{Locale: "en-US", Description: "Old description"}},
			}},
			{Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
				{ID: "existing-loc", Attributes: asc.AppStoreVersionLocalizationAttributes{Locale: "en-US", Description: "New description"}},
			}},
		},
		updateErrs: []error{&asc.RetryableError{Err: errors.New("ambiguous timeout")}},
	}

	results, err := UploadVersionLocalizations(context.Background(), client, "version-id", map[string]map[string]string{
		"en-US": {"description": "New description"},
	}, false)
	if err != nil {
		t.Fatalf("UploadVersionLocalizations() error: %v", err)
	}
	if len(client.updateCalls) != 1 {
		t.Fatalf("expected one update before readback, got %d", len(client.updateCalls))
	}
	if client.getCalls != 2 {
		t.Fatalf("expected initial read and reconciliation read, got %d", client.getCalls)
	}
	if len(results) != 1 || results[0].Action != "reconcile" || results[0].LocalizationID != "existing-loc" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestUploadVersionLocalizations_UsesFreshContextForReadbackAfterRequestTimeout(t *testing.T) {
	t.Setenv("ASC_TIMEOUT", "20ms")
	client := &expiringVersionLocalizationClient{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	results, err := UploadVersionLocalizations(ctx, client, "version-id", map[string]map[string]string{
		"en-US": {"description": "New description"},
	}, false)
	if err != nil {
		t.Fatalf("UploadVersionLocalizations() error: %v", err)
	}
	if client.updateCalls != 1 || client.getCalls != 2 {
		t.Fatalf("expected one timed-out update and a fresh readback, updates=%d reads=%d", client.updateCalls, client.getCalls)
	}
	if len(results) != 1 || results[0].Action != "reconcile" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestUploadVersionLocalizations_RetriesAfterNegativeReadback(t *testing.T) {
	t.Setenv("ASC_MAX_RETRIES", "1")
	t.Setenv("ASC_BASE_DELAY", "1ms")
	t.Setenv("ASC_MAX_DELAY", "1ms")
	client := &stubVersionLocalizationClient{
		getResps: []*asc.AppStoreVersionLocalizationsResponse{
			{Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
				{ID: "existing-loc", Attributes: asc.AppStoreVersionLocalizationAttributes{Locale: "en-US", Description: "Old description"}},
			}},
			{Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
				{ID: "existing-loc", Attributes: asc.AppStoreVersionLocalizationAttributes{Locale: "en-US", Description: "Old description"}},
			}},
			{Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
				{ID: "existing-loc", Attributes: asc.AppStoreVersionLocalizationAttributes{Locale: "en-US", Description: "Old description"}},
			}},
		},
		updateErrs: []error{&asc.RetryableError{Err: errors.New("temporary failure")}},
	}

	results, err := UploadVersionLocalizations(context.Background(), client, "version-id", map[string]map[string]string{
		"en-US": {"description": "New description"},
	}, false)
	if err != nil {
		t.Fatalf("UploadVersionLocalizations() error: %v", err)
	}
	if len(client.updateCalls) != 2 || client.getCalls != 3 {
		t.Fatalf("expected negative readback before one replay, updates=%d reads=%d", len(client.updateCalls), client.getCalls)
	}
	if len(results) != 1 || results[0].Action != "update" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestUploadVersionLocalizations_ContinuesAfterLocaleFailure(t *testing.T) {
	client := &stubVersionLocalizationClient{
		getResp: &asc.AppStoreVersionLocalizationsResponse{Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
			{ID: "en-id", Attributes: asc.AppStoreVersionLocalizationAttributes{Locale: "en-US", Description: "Old English"}},
			{ID: "fr-id", Attributes: asc.AppStoreVersionLocalizationAttributes{Locale: "fr-FR", Description: "Old French"}},
		}},
		updateErrs: []error{errors.New("english update rejected")},
	}

	results, err := UploadVersionLocalizations(context.Background(), client, "version-id", map[string]map[string]string{
		"en-US": {"description": "New English"},
		"fr-FR": {"description": "New French"},
	}, false)
	if err == nil {
		t.Fatal("expected partial batch error")
	}
	if len(results) != 2 || len(client.updateCalls) != 2 {
		t.Fatalf("expected both locales to be attempted, results=%+v updates=%d", results, len(client.updateCalls))
	}
	if results[0].Locale != "en-US" || results[0].Status != "failed" || results[0].Error == "" || results[0].DesiredValues["description"] != "New English" {
		t.Fatalf("unexpected failed result: %+v", results[0])
	}
	if results[1].Locale != "fr-FR" || results[1].Status != "succeeded" || results[1].Error != "" {
		t.Fatalf("unexpected successful result: %+v", results[1])
	}
}

func TestFinalizeLocalizationUploadResultWritesResumableArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	result := &asc.LocalizationUploadResult{
		Type:      LocalizationTypeVersion,
		VersionID: "version-id",
		InputPath: "localizations",
		Results: []asc.LocalizationUploadLocaleResult{
			{Locale: "en-US", Action: "update", Status: "failed", LocalizationID: "loc-id", Error: "timeout", DesiredValues: map[string]string{"description": "Desired"}},
			{Locale: "fr-FR", Action: "skip", Status: "succeeded", LocalizationID: "fr-id"},
		},
	}

	FinalizeLocalizationUploadResult(result, "localizations upload")
	if result.Total != 2 || result.Succeeded != 1 || result.Failed != 1 || result.FailureArtifactPath == "" || result.FailureArtifactError != "" {
		t.Fatalf("unexpected finalized result: %+v", result)
	}
	payload, err := os.ReadFile(result.FailureArtifactPath)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	var artifact localizationUploadFailureArtifact
	if err := json.Unmarshal(payload, &artifact); err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	if artifact.SchemaVersion != 1 || artifact.InputPath != "localizations" || len(artifact.Results) != 1 || artifact.Results[0].DesiredValues["description"] != "Desired" {
		t.Fatalf("artifact is not resumable: %+v", artifact)
	}
}

func TestFinalizeLocalizationUploadResultRetainsArtifactWriteError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".asc", []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	result := &asc.LocalizationUploadResult{
		Results: []asc.LocalizationUploadLocaleResult{{Locale: "en-US", Action: "create", Status: "failed", Error: "timeout"}},
	}

	FinalizeLocalizationUploadResult(result, "localizations upload")
	if result.Total != 1 || result.Failed != 1 || result.FailureArtifactPath != "" || result.FailureArtifactError == "" {
		t.Fatalf("expected artifact write error in summary: %+v", result)
	}
}

func TestRunLocalizationMutationWithReadbackRetriesChildDeadline(t *testing.T) {
	t.Setenv("ASC_MAX_RETRIES", "1")
	t.Setenv("ASC_BASE_DELAY", "1ms")
	t.Setenv("ASC_MAX_DELAY", "1ms")
	mutations := 0
	readbacks := 0

	id, reconciled, err := runLocalizationMutationWithReadback(
		context.Background(),
		func(context.Context) (string, error) {
			mutations++
			if mutations == 1 {
				return "", context.DeadlineExceeded
			}
			return "existing-loc", nil
		},
		func(context.Context) (string, bool, error) {
			readbacks++
			return "", false, nil
		},
	)
	if err != nil {
		t.Fatalf("runLocalizationMutationWithReadback() error: %v", err)
	}
	if id != "existing-loc" || reconciled || mutations != 2 || readbacks != 2 {
		t.Fatalf("unexpected recovery: id=%q reconciled=%t mutations=%d readbacks=%d", id, reconciled, mutations, readbacks)
	}
}

func TestRunLocalizationMutationWithReadbackStopsOnParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mutations := 0
	readbacks := 0

	_, _, err := runLocalizationMutationWithReadback(
		ctx,
		func(context.Context) (string, error) {
			mutations++
			return "", context.DeadlineExceeded
		},
		func(context.Context) (string, bool, error) {
			readbacks++
			return "", false, nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if mutations != 0 || readbacks != 0 {
		t.Fatalf("expected no requests after parent cancellation, mutations=%d readbacks=%d", mutations, readbacks)
	}
}

func (s *stubVersionLocalizationClient) UpdateAppStoreVersionLocalization(_ context.Context, _ string, attrs asc.AppStoreVersionLocalizationAttributes) (*asc.AppStoreVersionLocalizationResponse, error) {
	s.updateCalls = append(s.updateCalls, attrs)
	callIndex := len(s.updateCalls) - 1
	if callIndex < len(s.updateErrs) && s.updateErrs[callIndex] != nil {
		return nil, s.updateErrs[callIndex]
	}
	return &asc.AppStoreVersionLocalizationResponse{
		Data: asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
			ID: "existing-loc",
		},
	}, nil
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	defer func() {
		os.Stderr = oldStderr
	}()

	os.Stderr = writer
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		_ = reader.Close()
		done <- buf.String()
	}()

	fn()
	_ = writer.Close()

	return <-done
}

func TestUploadVersionLocalizations_RetriesWithoutWhatsNewOnInitialReleaseError(t *testing.T) {
	client := &stubVersionLocalizationClient{
		getResp: &asc.AppStoreVersionLocalizationsResponse{
			Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
				{
					ID: "existing-loc",
					Attributes: asc.AppStoreVersionLocalizationAttributes{
						Locale: "en-US",
					},
				},
			},
		},
		updateErrs: []error{
			errors.New("An attribute value is not acceptable for the current resource state. The attribute 'whatsNew' is not editable."),
		},
	}

	valuesByLocale := map[string]map[string]string{
		"en-US": {
			"description": "A description",
			"whatsNew":    "Bug fixes and improvements",
		},
	}

	var (
		results []asc.LocalizationUploadLocaleResult
		err     error
	)
	stderr := captureStderr(t, func() {
		results, err = UploadVersionLocalizations(context.Background(), client, "version-id", valuesByLocale, false)
	})
	if err != nil {
		t.Fatalf("UploadVersionLocalizations() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(client.updateCalls) != 2 {
		t.Fatalf("expected 2 update attempts, got %d", len(client.updateCalls))
	}
	if got := client.updateCalls[0].WhatsNew; got != "Bug fixes and improvements" {
		t.Fatalf("expected first attempt to include whatsNew, got %q", got)
	}
	if got := client.updateCalls[1].WhatsNew; got != "" {
		t.Fatalf("expected retry attempt to clear whatsNew, got %q", got)
	}
	if !strings.Contains(stderr, "Retrying without it") {
		t.Fatalf("expected retry warning in stderr, got %q", stderr)
	}
}

func TestUploadVersionLocalizations_DoesNotRetryWhenErrorIsUnrelated(t *testing.T) {
	client := &stubVersionLocalizationClient{
		getResp: &asc.AppStoreVersionLocalizationsResponse{
			Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
				{
					ID: "existing-loc",
					Attributes: asc.AppStoreVersionLocalizationAttributes{
						Locale: "en-US",
					},
				},
			},
		},
		updateErrs: []error{errors.New("network timeout")},
	}

	valuesByLocale := map[string]map[string]string{
		"en-US": {
			"description": "A description",
			"whatsNew":    "Bug fixes and improvements",
		},
	}

	_, err := UploadVersionLocalizations(context.Background(), client, "version-id", valuesByLocale, false)
	if err == nil {
		t.Fatal("expected upload error")
	}
	if len(client.updateCalls) != 1 {
		t.Fatalf("expected a single update attempt, got %d", len(client.updateCalls))
	}
}

func TestUploadVersionLocalizations_DoesNotRetryWhenWhatsNewIsEmpty(t *testing.T) {
	client := &stubVersionLocalizationClient{
		getResp: &asc.AppStoreVersionLocalizationsResponse{
			Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
				{
					ID: "existing-loc",
					Attributes: asc.AppStoreVersionLocalizationAttributes{
						Locale: "en-US",
					},
				},
			},
		},
		updateErrs: []error{
			errors.New("The attribute 'whatsNew' is not editable for this version"),
		},
	}

	valuesByLocale := map[string]map[string]string{
		"en-US": {
			"description": "A description",
		},
	}

	_, err := UploadVersionLocalizations(context.Background(), client, "version-id", valuesByLocale, false)
	if err == nil {
		t.Fatal("expected upload error")
	}
	if len(client.updateCalls) != 1 {
		t.Fatalf("expected a single update attempt, got %d", len(client.updateCalls))
	}
}

func TestUploadVersionLocalizationsWithWarnings_RejectsOverLimitKeywordCharactersBeforeFetch(t *testing.T) {
	client := &stubVersionLocalizationClient{}

	valuesByLocale := map[string]map[string]string{
		"ja": {
			"description": "日本語説明",
			"keywords":    strings.Repeat("語", 101),
		},
	}

	_, _, err := UploadVersionLocalizationsWithWarnings(context.Background(), client, "version-id", valuesByLocale, true, SubmitReadinessOptions{})
	if err == nil {
		t.Fatal("expected upload validation error")
	}
	if !strings.Contains(err.Error(), "keywords exceed 100 characters") {
		t.Fatalf("expected keyword character-limit error, got %v", err)
	}
	if client.getCalls != 0 {
		t.Fatalf("expected no fetch calls, got %d", client.getCalls)
	}
	if len(client.createCalls) != 0 {
		t.Fatalf("expected no create calls, got %d", len(client.createCalls))
	}
	if len(client.updateCalls) != 0 {
		t.Fatalf("expected no update calls, got %d", len(client.updateCalls))
	}
}

func TestUploadVersionLocalizationsWithWarnings_DryRunWarnsOnlyForCreates(t *testing.T) {
	client := &stubVersionLocalizationClient{
		getResp: &asc.AppStoreVersionLocalizationsResponse{
			Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{
				{
					ID: "existing-loc",
					Attributes: asc.AppStoreVersionLocalizationAttributes{
						Locale: "en-US",
					},
				},
			},
		},
	}

	valuesByLocale := map[string]map[string]string{
		"en-US": {
			"description": "Updated description",
			"keywords":    "updated",
			"supportUrl":  "https://example.com/en",
		},
		"ja": {
			"description": "日本語説明",
		},
	}

	results, warnings, err := UploadVersionLocalizationsWithWarnings(context.Background(), client, "version-id", valuesByLocale, true, SubmitReadinessOptions{})
	if err != nil {
		t.Fatalf("UploadVersionLocalizationsWithWarnings() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d (%+v)", len(warnings), warnings)
	}
	if warnings[0].Locale != "ja" {
		t.Fatalf("expected warning for ja locale, got %+v", warnings[0])
	}
	if warnings[0].Mode != SubmitReadinessCreateModePlanned {
		t.Fatalf("expected planned warning mode, got %+v", warnings[0])
	}
	if got := strings.Join(warnings[0].MissingFields, ","); got != "keywords,supportUrl" {
		t.Fatalf("unexpected missing fields %q", got)
	}
	if len(client.createCalls) != 0 {
		t.Fatalf("expected dry-run to avoid create calls, got %d", len(client.createCalls))
	}
	if len(client.updateCalls) != 0 {
		t.Fatalf("expected dry-run to avoid update calls, got %d", len(client.updateCalls))
	}
}

func TestUploadVersionLocalizationsWithWarnings_AppliedCompleteCreateDoesNotWarn(t *testing.T) {
	client := &stubVersionLocalizationClient{
		getResp: &asc.AppStoreVersionLocalizationsResponse{
			Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{},
		},
	}

	valuesByLocale := map[string]map[string]string{
		"ja": {
			"description": "日本語説明",
			"keywords":    "一,二",
			"supportUrl":  "https://example.com/ja",
		},
	}

	results, warnings, err := UploadVersionLocalizationsWithWarnings(context.Background(), client, "version-id", valuesByLocale, false, SubmitReadinessOptions{})
	if err != nil {
		t.Fatalf("UploadVersionLocalizationsWithWarnings() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", warnings)
	}
	if len(client.createCalls) != 1 {
		t.Fatalf("expected one create call, got %d", len(client.createCalls))
	}
}

func TestIsWhatsNewUnsupportedError(t *testing.T) {
	apiErr := &asc.APIError{
		Title:  "ENTITY_ERROR.ATTRIBUTE.INVALID",
		Detail: "The attribute 'whatsNew' cannot be set for this resource state.",
	}
	if !isWhatsNewUnsupportedError(apiErr) {
		t.Fatal("expected API error with whatsNew detail to be recognized")
	}
	if isWhatsNewUnsupportedError(errors.New("timeout")) {
		t.Fatal("did not expect unrelated error to match")
	}
}

func TestUploadVersionLocalizationsWithWarnings_RequiresWhatsNewWhenConfigured(t *testing.T) {
	client := &stubVersionLocalizationClient{
		getResp: &asc.AppStoreVersionLocalizationsResponse{
			Data: []asc.Resource[asc.AppStoreVersionLocalizationAttributes]{},
		},
	}

	valuesByLocale := map[string]map[string]string{
		"ja": {
			"description": "日本語説明",
			"keywords":    "一,二",
			"supportUrl":  "https://example.com/ja",
		},
	}

	results, warnings, err := UploadVersionLocalizationsWithWarnings(
		context.Background(),
		client,
		"version-id",
		valuesByLocale,
		true,
		SubmitReadinessOptions{RequireWhatsNew: true},
	)
	if err != nil {
		t.Fatalf("UploadVersionLocalizationsWithWarnings() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %+v", warnings)
	}
	if warnings[0].Locale != "ja" {
		t.Fatalf("expected warning for ja locale, got %+v", warnings[0])
	}
	if len(warnings[0].MissingFields) != 1 || warnings[0].MissingFields[0] != "whatsNew" {
		t.Fatalf("expected missing fields [whatsNew], got %+v", warnings[0].MissingFields)
	}
}
