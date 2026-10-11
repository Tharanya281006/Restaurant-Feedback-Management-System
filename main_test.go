package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	path := filepath.Join(t.TempDir(), "feedback-data.json")
	prev := os.Getenv("FEEDBACK_DATA_FILE")
	if err := os.Setenv("FEEDBACK_DATA_FILE", path); err != nil {
		t.Fatalf("set FEEDBACK_DATA_FILE: %v", err)
	}
	t.Cleanup(func() {
		if prev == "" {
			os.Unsetenv("FEEDBACK_DATA_FILE")
			return
		}
		_ = os.Setenv("FEEDBACK_DATA_FILE", prev)
	})
	handler, err := newHandler()
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}
	return handler
}

func multipartRequest(t *testing.T, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField(%q): %v", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/feedback", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func validFields() map[string]string {
	fields := map[string]string{
		"name":       "Asha",
		"mobile":     "+1 555 123 4567",
		"email":      "asha@example.com",
		"anonymous":  "false",
		"experience": "good",
		"comment":    "The meal was lovely and the team was welcoming.",
	}
	for i := range categories {
		fields["rating_"+strconv.Itoa(i)] = "5"
	}
	return fields
}

func TestFeedbackSubmissionTrackingAndResponse(t *testing.T) {
	handler := newTestHandler(t)

	request := multipartRequest(t, validFields())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("submission response = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}

	var created Feedback
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode submission response: %v", err)
	}
	if !strings.HasPrefix(created.Reference, "FB-") || created.Status != "Submitted" {
		t.Fatalf("created feedback = %#v, want reference and Submitted status", created)
	}

	statusURL := "/api/feedback/status?reference=" + url.QueryEscape(created.Reference)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, statusURL, nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status without mobile = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	statusURL += "&mobile=" + url.QueryEscape("+1 555 123 4567")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, statusURL, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "The meal was lovely") {
		t.Fatalf("verified status response = %d: %s", response.Code, response.Body.String())
	}
}

func TestAnonymousFeedbackDoesNotRetainIdentity(t *testing.T) {
	handler := newTestHandler(t)
	fields := validFields()
	fields["anonymous"] = "true"
	request := multipartRequest(t, fields)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("submission response = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}

	var created Feedback
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode submission response: %v", err)
	}
	if !created.Anonymous || created.Name != "" || created.Mobile != "" || created.Email != "" {
		t.Fatalf("anonymous feedback retained identity: %#v", created)
	}
}

func TestSubmissionRejectsMissingCategoryRating(t *testing.T) {
	handler := newTestHandler(t)
	fields := validFields()
	delete(fields, "rating_3")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, multipartRequest(t, fields))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Please rate cleanliness") {
		t.Fatalf("invalid submission response = %d: %s", response.Code, response.Body.String())
	}
}

func TestStaffListReturnsAllFeedback(t *testing.T) {
	prev := os.Getenv("FEEDBACK_DATA_FILE")
	path := filepath.Join(t.TempDir(), "feedback-data.json")
	if err := os.Setenv("FEEDBACK_DATA_FILE", path); err != nil {
		t.Fatalf("set FEEDBACK_DATA_FILE: %v", err)
	}
	t.Cleanup(func() {
		if prev == "" {
			os.Unsetenv("FEEDBACK_DATA_FILE")
			return
		}
		_ = os.Setenv("FEEDBACK_DATA_FILE", prev)
	})
	store := newFeedbackStore()
	first := Feedback{
		Name:       "Asha",
		Mobile:     "5551234567",
		Experience: "good",
		Ratings:    make(map[string]int),
		Comment:    "The meal was lovely and the team was welcoming.",
		Status:     "Submitted",
		Submitted:  time.Now().Add(-time.Hour),
	}
	for _, category := range categories {
		first.Ratings[category] = 4
	}
	if _, err := store.create(first); err != nil {
		t.Fatalf("store.create(first) error = %v", err)
	}

	second := Feedback{
		Name:       "Rahul",
		Mobile:     "5557654321",
		Experience: "poor",
		Ratings:    make(map[string]int),
		Comment:    "The wait time was too long and the staff seemed rushed.",
		Status:     "Under Review",
		Submitted:  time.Now(),
	}
	for _, category := range categories {
		second.Ratings[category] = 2
	}
	if _, err := store.create(second); err != nil {
		t.Fatalf("store.create(second) error = %v", err)
	}

	server := &feedbackServer{store: store, staffKey: "test-key"}
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/staff/feedback", server.staffList)

	request := httptest.NewRequest(http.MethodGet, "/api/staff/feedback", nil)
	request.Header.Set("X-Staff-Key", "test-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("staff listing response = %d: %s", response.Code, response.Body.String())
	}

	var list []feedbackListItem
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode staff list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("staff list size = %d, want 2", len(list))
	}
	if list[0].Name == "" || list[1].Name == "" {
		t.Fatal("staff list returned feedback without names")
	}
	if strings.Contains(response.Body.String(), "reference") {
		t.Fatal("staff list included a reference field")
	}
}

func TestStaffResponseRequiresKeyAndMovesStatus(t *testing.T) {
	prev := os.Getenv("FEEDBACK_DATA_FILE")
	path := filepath.Join(t.TempDir(), "feedback-data.json")
	if err := os.Setenv("FEEDBACK_DATA_FILE", path); err != nil {
		t.Fatalf("set FEEDBACK_DATA_FILE: %v", err)
	}
	t.Cleanup(func() {
		if prev == "" {
			os.Unsetenv("FEEDBACK_DATA_FILE")
			return
		}
		_ = os.Setenv("FEEDBACK_DATA_FILE", prev)
	})
	store := newFeedbackStore()
	feedback := Feedback{
		Name:       "Asha",
		Mobile:     "5551234567",
		Experience: "poor",
		Ratings:    make(map[string]int),
		Comment:    "The wait was much too long.",
		Status:     "Submitted",
	}
	for _, category := range categories {
		feedback.Ratings[category] = 2
	}
	created, err := store.create(feedback)
	if err != nil {
		t.Fatalf("store.create() error = %v", err)
	}
	server := &feedbackServer{store: store, staffKey: "test-key"}
	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/staff/feedback/{reference}", server.staffResponse)

	update := `{"status":"Under Review"}`
	request := httptest.NewRequest(http.MethodPost, "/api/staff/feedback/"+created.Reference, strings.NewReader(update))
	request.Header.Set("X-Staff-Key", "test-key")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("review update response = %d: %s", response.Code, response.Body.String())
	}

	update = `{"response":"We are sorry and have spoken with the team."}`
	request = httptest.NewRequest(http.MethodPost, "/api/staff/feedback/"+created.Reference, strings.NewReader(update))
	request.Header.Set("X-Staff-Key", "wrong-key")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("response update without staff key = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/staff/feedback/"+created.Reference, strings.NewReader(update))
	request.Header.Set("X-Staff-Key", "test-key")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"Responded"`) {
		t.Fatalf("response update = %d: %s", response.Code, response.Body.String())
	}
}

func TestQRCodeAndCustomerPage(t *testing.T) {
	handler := newTestHandler(t)
	for path, contentType := range map[string]string{
		"/":       "text/html",
		"/qr.png": "image/png",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), contentType) {
			t.Errorf("GET %s = %d, Content-Type %q", path, response.Code, response.Header().Get("Content-Type"))
		}
	}
}

func TestPublicFeedbackSummaryAggregatesWithoutPersonalDetails(t *testing.T) {
	handler := newTestHandler(t)
	request := multipartRequest(t, validFields())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("submission response = %d: %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/feedback/summary", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("summary response = %d: %s", response.Code, response.Body.String())
	}
	var summary feedbackSummary
	if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.Count != 1 || summary.Average != 5 || summary.CategoryAverages["Service"] != 5 || summary.Experience["good"] != 1 {
		t.Fatalf("unexpected aggregated summary: %#v", summary)
	}
	if strings.Contains(response.Body.String(), "Asha") || strings.Contains(response.Body.String(), "The meal was lovely") || strings.Contains(response.Body.String(), "+1 555") {
		t.Fatal("public summary exposed personal details or an individual comment")
	}
}
