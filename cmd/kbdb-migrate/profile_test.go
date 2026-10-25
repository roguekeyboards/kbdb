package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ProfileSuite struct {
	suite.Suite

	profile map[string]any
	avatar  []byte
	calls   []string
	srv     *httptest.Server
	client  *apiClient
}

func TestProfileSuite(t *testing.T) {
	suite.Run(t, new(ProfileSuite))
}

func (s *ProfileSuite) SetupTest() {
	s.profile, s.avatar, s.calls = nil, nil, nil
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	s.client = &apiClient{baseURL: s.srv.URL, subject: "u-1", http: s.srv.Client()}
}

func (s *ProfileSuite) TearDownTest() {
	s.srv.Close()
}

func (s *ProfileSuite) serve(w http.ResponseWriter, r *http.Request) {
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	switch {
	case r.URL.Path == "/v1/profile/u-1" && r.Method == http.MethodGet:
		if s.profile == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body := maps.Clone(s.profile)
		if s.avatar != nil {
			body["avatar"] = map[string]any{"url": s.srv.URL + "/avatar", "status": "pending"}
		}
		_ = json.NewEncoder(w).Encode(body)
	case r.URL.Path == "/v1/profile/u-1" && r.Method == http.MethodPost:
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["username"] == "taken" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		in["user_id"] = "u-1"
		s.profile = in
		w.WriteHeader(http.StatusCreated)
	case r.URL.Path == "/v1/profile/u-1" && r.Method == http.MethodPut:
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		in["user_id"] = "u-1"
		s.profile = in
	case r.URL.Path == "/v1/profile/u-1/image":
		_, _ = fmt.Fprintf(w, `{"upload_url":%q}`, s.srv.URL+"/avatar")
	case r.URL.Path == "/avatar" && r.Method == http.MethodPut:
		s.avatar, _ = io.ReadAll(r.Body)
	case r.URL.Path == "/avatar":
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(s.avatar)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *ProfileSuite) writeDump(profile string, avatar []byte) string {
	dumpDir := s.T().TempDir()
	dir := filepath.Join(dumpDir, profileDir)
	s.Require().NoError(os.MkdirAll(dir, 0o750))
	s.Require().NoError(os.WriteFile(filepath.Join(dir, "profile.json"), []byte(profile), 0o600))
	if avatar != nil {
		s.Require().NoError(os.WriteFile(filepath.Join(dir, "avatar.webp"), avatar, 0o600))
		manifest, err := json.Marshal(imageManifestEntry{
			Filename: "avatar.webp", ContentType: "image/webp", Bytes: len(avatar), SHA256: sha256Hex(avatar),
		})
		s.Require().NoError(err)
		s.Require().NoError(os.WriteFile(filepath.Join(dir, "avatar.manifest.json"), manifest, 0o600))
	}
	return dumpDir
}

const dumpedProfile = `{"user_id":"u-old","username":"jay","discoverable":true,"bio":"hi",` +
	`"links":[{"name":"Site","url":"https://example.com"}],` +
	`"preferences":{"currency":"EUR","show_price_to_me":true,"show_price_to_others":false}}`

func (s *ProfileSuite) TestDumpProfile_WritesProfileAndAvatar() {
	s.profile = map[string]any{"user_id": "u-1", "username": "jay", "discoverable": true}
	s.avatar = []byte("avatar bytes")
	outDir := s.T().TempDir()

	found, images, err := dumpProfile(context.Background(), s.client, outDir)

	s.Require().NoError(err)
	s.True(found)
	s.Equal(1, images)
	s.FileExists(filepath.Join(outDir, profileDir, "profile.json"))
	got, err := os.ReadFile(filepath.Join(outDir, profileDir, "avatar.webp")) //nolint:gosec // test temp dir.
	s.Require().NoError(err)
	s.Equal(s.avatar, got)
}

func (s *ProfileSuite) TestDumpProfile_NoProfileWritesNothing() {
	outDir := s.T().TempDir()

	found, images, err := dumpProfile(context.Background(), s.client, outDir)

	s.Require().NoError(err)
	s.False(found)
	s.Zero(images)
	s.NoDirExists(filepath.Join(outDir, profileDir))
}

func (s *ProfileSuite) TestRestoreProfile_CreatesWithDumpedUsernameAndAvatar() {
	avatar := []byte("avatar bytes")
	dumpDir := s.writeDump(dumpedProfile, avatar)

	err := restoreProfile(context.Background(), s.client, dumpDir)

	s.Require().NoError(err)
	s.Contains(s.calls, "POST /v1/profile/u-1")
	s.Equal("jay", s.profile["username"])
	s.Equal("hi", s.profile["bio"])
	s.Equal(avatar, s.avatar)
}

func (s *ProfileSuite) TestRestoreProfile_ExistingProfileKeepsItsUsername() {
	s.profile = map[string]any{"user_id": "u-1", "username": "someone", "discoverable": false}
	dumpDir := s.writeDump(dumpedProfile, nil)

	err := restoreProfile(context.Background(), s.client, dumpDir)

	s.Require().NoError(err)
	s.Contains(s.calls, "PUT /v1/profile/u-1")
	s.Equal("someone", s.profile["username"])
	s.Equal("hi", s.profile["bio"])
	s.Equal(true, s.profile["discoverable"])
}

func (s *ProfileSuite) TestRestoreProfile_TakenUsernameIsError() {
	dumpDir := s.writeDump(`{"user_id":"u-old","username":"taken","discoverable":false}`, nil)

	err := restoreProfile(context.Background(), s.client, dumpDir)

	s.Require().ErrorContains(err, "nothing has been restored")
}

func (s *ProfileSuite) TestRestoreProfile_ResumeSkipsMatchingAvatar() {
	avatar := []byte("avatar bytes")
	s.profile = map[string]any{"user_id": "u-1", "username": "jay", "discoverable": true}
	s.avatar = avatar
	dumpDir := s.writeDump(dumpedProfile, avatar)

	err := restoreProfile(context.Background(), s.client, dumpDir)

	s.Require().NoError(err)
	s.NotContains(s.calls, "POST /v1/profile/u-1/image")
}

func (s *ProfileSuite) TestRestoreProfile_NoDumpedProfileDoesNothing() {
	err := restoreProfile(context.Background(), s.client, s.T().TempDir())

	s.Require().NoError(err)
	s.Empty(s.calls)
}

func (s *ProfileSuite) TestVerifyProfile_MatchIgnoresUsernameAndUserID() {
	avatar := []byte("avatar bytes")
	s.Require().NoError(json.Unmarshal([]byte(dumpedProfile), &s.profile))
	s.profile["user_id"], s.profile["username"] = "u-1", "someone"
	s.avatar = avatar
	dumpDir := s.writeDump(dumpedProfile, avatar)

	results := verifyProfile(context.Background(), s.client, dumpDir)

	s.Require().Len(results, 1)
	s.Equal(statusOK, results[0].status, results[0].detail)
}

func (s *ProfileSuite) TestVerifyProfile_FieldDiffersIsMismatch() {
	s.Require().NoError(json.Unmarshal([]byte(dumpedProfile), &s.profile))
	s.profile["bio"] = "changed"
	dumpDir := s.writeDump(dumpedProfile, nil)

	results := verifyProfile(context.Background(), s.client, dumpDir)

	s.Require().Len(results, 1)
	s.Equal(statusFieldMismatch, results[0].status)
}

func (s *ProfileSuite) TestVerifyProfile_MissingAvatarIsMismatch() {
	s.Require().NoError(json.Unmarshal([]byte(dumpedProfile), &s.profile))
	dumpDir := s.writeDump(dumpedProfile, []byte("avatar bytes"))

	results := verifyProfile(context.Background(), s.client, dumpDir)

	s.Require().Len(results, 1)
	s.Equal(statusImageCountMismatch, results[0].status)
}
