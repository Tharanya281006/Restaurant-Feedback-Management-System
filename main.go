package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Feedback struct {
	Reference  string         `json:"reference"`
	Name       string         `json:"name,omitempty"`
	Mobile     string         `json:"mobile,omitempty"`
	Email      string         `json:"email,omitempty"`
	Anonymous  bool           `json:"anonymous"`
	Type       string         `json:"type,omitempty"`
	VisitDate  string         `json:"visitDate,omitempty"`
	Rating     int            `json:"rating"`
	Experience string         `json:"experience,omitempty"`
	Ratings    map[string]int `json:"ratings,omitempty"`
	Comment    string         `json:"comment"`
	Issue      bool           `json:"issue,omitempty"`
	Status     string         `json:"status"`
	Response   string         `json:"response,omitempty"`
	Submitted  time.Time      `json:"submittedAt"`
}

type feedbackStore struct {
	mu   sync.RWMutex
	path string
	data map[string]Feedback
}

func newFeedbackStore() *feedbackStore {
	path := os.Getenv("FEEDBACK_DATA_FILE")
	if path == "" {
		path = "feedback-data.json"
	}
	store := &feedbackStore{path: path, data: make(map[string]Feedback)}
	_ = store.load()
	return store
}

func (s *feedbackStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &s.data)
}

func (s *feedbackStore) saveLocked() error {
	dir := ""
	if idx := strings.LastIndexAny(s.path, `/\\`); idx >= 0 {
		dir = s.path[:idx]
		if dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
		}
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *feedbackStore) create(f Feedback) (Feedback, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.Reference = newReference()
	f.Submitted = time.Now()
	f.Status = "Submitted"
	s.data[f.Reference] = f
	if err := s.saveLocked(); err != nil {
		delete(s.data, f.Reference)
		return Feedback{}, err
	}
	return f, nil
}

func (s *feedbackStore) all() []Feedback {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Feedback, 0, len(s.data))
	for _, f := range s.data {
		list = append(list, normalizeFeedback(f))
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Submitted.After(list[j].Submitted) })
	return list
}

func normalizeFeedback(f Feedback) Feedback {
	if f.Anonymous {
		f.Name, f.Mobile, f.Email = "", "", ""
	}
	if f.Rating < 1 && len(f.Ratings) > 0 {
		total, count := 0, 0
		for _, rating := range f.Ratings {
			if rating > 0 {
				total += rating
				count++
			}
		}
		if count > 0 {
			f.Rating = int(float64(total)/float64(count) + 0.5)
		}
	}
	if f.Status == "" {
		f.Status = "Submitted"
	}
	return f
}

func (s *feedbackStore) find(reference string) (Feedback, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.data[reference]
	return normalizeFeedback(f), ok
}

func newReference() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("FB-%d", time.Now().UnixNano())
	}
	return "FB-" + time.Now().Format("20060102") + "-" + strings.ToUpper(hex.EncodeToString(b))
}

type feedbackServer struct {
	store *feedbackStore
}

func feedbackPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, "Unable to load page", http.StatusInternalServerError)
		return
	}
	_ = tmpl.Execute(w, nil)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *feedbackServer) createFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		if err := r.ParseForm(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form data."})
			return
		}
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	mobile := strings.TrimSpace(r.FormValue("mobile"))
	feedbackType := strings.TrimSpace(r.FormValue("type"))
	visitDate := strings.TrimSpace(r.FormValue("visitDate"))
	comment := strings.TrimSpace(r.FormValue("comment"))

	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Please enter the customer name."})
		return
	}
	if comment == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Please enter your feedback."})
		return
	}
	if len([]rune(comment)) > 1200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Feedback must be 1,200 characters or fewer."})
		return
	}

	rating, err := strconv.Atoi(strings.TrimSpace(r.FormValue("rating")))
	if err != nil || rating < 1 || rating > 5 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Please select a rating from 1 to 5."})
		return
	}

	categoryFields := map[string]string{
		"food": "foodRating", "service": "serviceRating", "staff": "staffRating",
		"cleanliness": "cleanlinessRating", "ambience": "ambienceRating", "value": "valueRating",
	}
	categoryRatings := make(map[string]int, len(categoryFields))
	providedCategoryRatings := 0
	for _, field := range categoryFields {
		if strings.TrimSpace(r.FormValue(field)) != "" {
			providedCategoryRatings++
		}
	}
	if providedCategoryRatings != 0 && providedCategoryRatings != len(categoryFields) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Please rate all six restaurant experience areas from 1 to 5."})
		return
	}
	for category, field := range categoryFields {
		if providedCategoryRatings == 0 { // Keep compatibility with older saved-form/API clients.
			break
		}
		value, parseErr := strconv.Atoi(strings.TrimSpace(r.FormValue(field)))
		if parseErr != nil || value < 1 || value > 5 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Please rate all six restaurant experience areas from 1 to 5."})
			return
		}
		categoryRatings[category] = value
	}

	f := Feedback{
		Anonymous: false,
		Name:      name,
		Mobile:    mobile,
		Email:     email,
		Type:      feedbackType,
		VisitDate: visitDate,
		Rating:    rating,
		Ratings:   categoryRatings,
		Comment:   comment,
	}
	created, err := s.store.create(f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Could not save feedback."})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *feedbackServer) allFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.store.all())
}

func (s *feedbackServer) summary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list := s.store.all()
	total := 0
	counts := [6]int{}
	for _, f := range list {
		total += f.Rating
		if f.Rating >= 1 && f.Rating <= 5 {
			counts[f.Rating]++
		}
	}
	average := 0.0
	if len(list) > 0 {
		average = float64(total) / float64(len(list))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":   len(list),
		"average": average,
		"ratings": counts[1:],
	})
}

func (s *feedbackServer) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	reference := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("reference")))
	if reference == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Reference is required."})
		return
	}
	f, ok := s.store.find(reference)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Feedback not found."})
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func main() {
	store := newFeedbackStore()
	server := &feedbackServer{store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("/", feedbackPage)
	mux.HandleFunc("/api/feedback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			server.createFeedback(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/api/feedback/all", server.allFeedback)
	mux.HandleFunc("/api/feedback/summary", server.summary)
	mux.HandleFunc("/api/feedback/status", server.status)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	fmt.Println("Feedback System running at http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Println("Server error:", err)
	}
	_ = io.Discard
}
