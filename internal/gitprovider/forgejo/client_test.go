package forgejo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lirancohen/dex/internal/gitprovider"
)

func TestClient_Name(t *testing.T) {
	c := New("http://localhost:3000", "test-token")
	if got := c.Name(); got != "forgejo" {
		t.Errorf("Name() = %q, want %q", got, "forgejo")
	}
}

func TestClient_Ping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/version" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "token test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"1.21.0"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}

func TestClient_CreateIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/repos/myorg/myrepo/issues" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["title"] != "Test Issue" {
			t.Errorf("title = %q, want %q", body["title"], "Test Issue")
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":42,"title":"Test Issue","body":"body","state":"open","created_at":"2025-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	issue, err := c.CreateIssue(context.Background(), "myorg", "myrepo", gitprovider.CreateIssueOpts{
		Title: "Test Issue",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if issue.Number != 42 {
		t.Errorf("Number = %d, want 42", issue.Number)
	}
	if issue.State != "open" {
		t.Errorf("State = %q, want %q", issue.State, "open")
	}
}

func TestClient_UpdateIssue_WithState(t *testing.T) {
	var receivedBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/repos/myorg/myrepo/issues/5" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")

	t.Run("state only", func(t *testing.T) {
		openState := "open"
		err := c.UpdateIssue(context.Background(), "myorg", "myrepo", 5, gitprovider.UpdateIssueOpts{
			State: &openState,
		})
		if err != nil {
			t.Fatalf("UpdateIssue() error = %v", err)
		}
		if receivedBody["state"] != "open" {
			t.Errorf("state = %v, want %q", receivedBody["state"], "open")
		}
		// Title and body should not be present
		if _, ok := receivedBody["title"]; ok {
			t.Error("title should not be in request body")
		}
		if _, ok := receivedBody["body"]; ok {
			t.Error("body should not be in request body")
		}
	})

	t.Run("title and body", func(t *testing.T) {
		title := "Updated Title"
		body := "Updated Body"
		err := c.UpdateIssue(context.Background(), "myorg", "myrepo", 5, gitprovider.UpdateIssueOpts{
			Title: &title,
			Body:  &body,
		})
		if err != nil {
			t.Fatalf("UpdateIssue() error = %v", err)
		}
		if receivedBody["title"] != "Updated Title" {
			t.Errorf("title = %v, want %q", receivedBody["title"], "Updated Title")
		}
		if receivedBody["body"] != "Updated Body" {
			t.Errorf("body = %v, want %q", receivedBody["body"], "Updated Body")
		}
	})

	t.Run("all fields", func(t *testing.T) {
		title := "New Title"
		body := "New Body"
		state := "closed"
		err := c.UpdateIssue(context.Background(), "myorg", "myrepo", 5, gitprovider.UpdateIssueOpts{
			Title: &title,
			Body:  &body,
			State: &state,
		})
		if err != nil {
			t.Fatalf("UpdateIssue() error = %v", err)
		}
		if receivedBody["title"] != "New Title" {
			t.Errorf("title = %v, want %q", receivedBody["title"], "New Title")
		}
		if receivedBody["state"] != "closed" {
			t.Errorf("state = %v, want %q", receivedBody["state"], "closed")
		}
	})
}

func TestClient_CloseIssue(t *testing.T) {
	var receivedBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	if err := c.CloseIssue(context.Background(), "myorg", "myrepo", 10); err != nil {
		t.Fatalf("CloseIssue() error = %v", err)
	}
	if receivedBody["state"] != "closed" {
		t.Errorf("state = %v, want %q", receivedBody["state"], "closed")
	}
}

func TestClient_AddComment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/myorg/myrepo/issues/7/comments" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["body"] != "Hello world" {
			t.Errorf("body = %v, want %q", body["body"], "Hello world")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":99,"body":"Hello world","created_at":"2025-01-01T00:00:00Z"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	comment, err := c.AddComment(context.Background(), "myorg", "myrepo", 7, "Hello world")
	if err != nil {
		t.Fatalf("AddComment() error = %v", err)
	}
	if comment.ID != 99 {
		t.Errorf("ID = %d, want 99", comment.ID)
	}
}

func TestClient_CreatePR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/myorg/myrepo/pulls" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["head"] != "feature-branch" {
			t.Errorf("head = %v, want %q", body["head"], "feature-branch")
		}
		if body["base"] != "main" {
			t.Errorf("base = %v, want %q", body["base"], "main")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"number": 3,
			"title": "My PR",
			"body": "PR body",
			"state": "open",
			"html_url": "http://localhost:3000/myorg/myrepo/pulls/3",
			"head": {"ref": "feature-branch"},
			"base": {"ref": "main"},
			"created_at": "2025-01-01T00:00:00Z"
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	pr, err := c.CreatePR(context.Background(), "myorg", "myrepo", gitprovider.CreatePROpts{
		Title: "My PR",
		Body:  "PR body",
		Head:  "feature-branch",
		Base:  "main",
	})
	if err != nil {
		t.Fatalf("CreatePR() error = %v", err)
	}
	if pr.Number != 3 {
		t.Errorf("Number = %d, want 3", pr.Number)
	}
	if pr.Head != "feature-branch" {
		t.Errorf("Head = %q, want %q", pr.Head, "feature-branch")
	}
}

func TestClient_MergePR(t *testing.T) {
	var receivedBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/myorg/myrepo/pulls/3/merge" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	if err := c.MergePR(context.Background(), "myorg", "myrepo", 3, gitprovider.MergeRebase); err != nil {
		t.Fatalf("MergePR() error = %v", err)
	}
	if receivedBody["Do"] != "rebase" {
		t.Errorf("Do = %v, want %q", receivedBody["Do"], "rebase")
	}
}

func TestClient_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestClient_CreateRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/orgs/myorg/repos" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["default_branch"] != "main" {
			t.Errorf("default_branch = %v, want %q", body["default_branch"], "main")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"owner": {"login": "myorg"},
			"name": "newrepo",
			"full_name": "myorg/newrepo",
			"clone_url": "http://localhost:3000/myorg/newrepo.git",
			"default_branch": "main",
			"private": true
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	repo, err := c.CreateRepo(context.Background(), "myorg", gitprovider.CreateRepoOpts{
		Name:    "newrepo",
		Private: true,
	})
	if err != nil {
		t.Fatalf("CreateRepo() error = %v", err)
	}
	if repo.Owner != "myorg" {
		t.Errorf("Owner = %q, want %q", repo.Owner, "myorg")
	}
	if repo.Name != "newrepo" {
		t.Errorf("Name = %q, want %q", repo.Name, "newrepo")
	}
	if !repo.Private {
		t.Error("Private = false, want true")
	}
}
