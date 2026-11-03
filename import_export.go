package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"muara.bi/desktop/internal/bundle"
)

// This file is the app half of the S5 Export/Import screen. The dialog-shaped
// methods (ExportBundle, ImportPick) own the OS file pickers for the same
// reason OpenWithDialog does: a path typed into a webview is a path the app
// would act on without the user having seen it (BRD §6). The _locked variants
// are the testable cores, separated from the dialogs and from the mutex.

// ExportRequest is the export dialog's answer: one scope (FR-10), the tier-2
// decision (§5.5), and an optional passphrase.
type ExportRequest struct {
	// Exactly one of these must be set; the refusal is Export's own.
	All         bool   `json:"all"`
	ViewID      string `json:"viewId,omitempty"`
	Folder      string `json:"folder,omitempty"`
	DashboardID string `json:"dashboardId,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`

	// IncludeServerDetails is the §5.5 tier-2 toggle. The dialog starts it ON;
	// false drops every Address/SSH block, which is the safe direction when a
	// caller never sets it.
	IncludeServerDetails bool `json:"includeServerDetails"`

	// Passphrase seals the tier-2 address blocks into secrets.enc instead of
	// leaving them in the clear. Empty means no sealing.
	Passphrase string `json:"passphrase,omitempty"`
}

// ExportResult is what the dialog shows after the save: where the file went
// and what it carries.
type ExportResult struct {
	// Cancelled is true when the user closed the save dialog without a name —
	// an answer, not a failure, and the open document is untouched.
	Cancelled bool   `json:"cancelled"`
	Path      string `json:"path,omitempty"`
	Name      string `json:"name,omitempty"`
	// Sealed is true when the passphrase path was taken: the address blocks
	// traveled only inside secrets.enc.
	Sealed bool `json:"sealed"`

	Projects       int `json:"projects"`
	Connections    int `json:"connections"`
	SemanticModels int `json:"semanticModels"`
	Views          int `json:"views"`
	Dashboards     int `json:"dashboards"`
	Alerts         int `json:"alerts"`
}

// ExportScopeOption is one entry a scope dropdown can name.
type ExportScopeOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ExportCatalog lists what the open document has, so the dialog offers
// concrete options instead of free text that could not resolve.
type ExportCatalog struct {
	Views      []ExportScopeOption `json:"views,omitempty"`
	Folders    []string            `json:"folders,omitempty"`
	Dashboards []ExportScopeOption `json:"dashboards,omitempty"`
	Projects   []ExportScopeOption `json:"projects,omitempty"`
}

// Catalog lists the scope options for the export dialog. Nothing here reads
// source data: names and ids only.
func (s *QueryService) Catalog() (*ExportCatalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cat := &ExportCatalog{}
	if s.doc == nil {
		return cat, nil
	}
	b := s.doc.bundle
	folders := map[string]bool{}
	for _, v := range b.Views {
		cat.Views = append(cat.Views, ExportScopeOption{ID: v.ID, Name: v.Name})
		if v.Folder != "" {
			folders[v.Folder] = true
		}
	}
	for f := range folders {
		cat.Folders = append(cat.Folders, f)
	}
	sort.Strings(cat.Folders)
	for _, d := range b.Dashboards {
		cat.Dashboards = append(cat.Dashboards, ExportScopeOption{ID: d.ID, Name: d.Name})
	}
	for _, p := range b.Projects {
		cat.Projects = append(cat.Projects, ExportScopeOption{ID: p.ID, Name: p.Name})
	}
	return cat, nil
}

// ExportPreviewResult names what an export would carry, so the dialog can
// show the dependency closure before the user commits to a file name.
type ExportPreviewResult struct {
	Projects       []string `json:"projects,omitempty"`
	Connections    []string `json:"connections,omitempty"`
	SemanticModels []string `json:"semanticModels,omitempty"`
	Views          []string `json:"views,omitempty"`
	Dashboards     []string `json:"dashboards,omitempty"`
	Alerts        []string `json:"alerts,omitempty"`
}

// ExportPreview computes the same closure Export does, without writing
// anything: the dialog renders it so the user sees what will travel.
func (s *QueryService) ExportPreview(req ExportRequest) (*ExportPreviewResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exportPreviewLocked(req)
}

func (s *QueryService) exportPreviewLocked(req ExportRequest) (*ExportPreviewResult, error) {
	if s.doc == nil {
		return nil, fmt.Errorf("no bundle is open — make one with NewUntitled or open a file first")
	}
	exported, err := s.doc.bundle.Export(bundle.Scope{
		All:         req.All,
		ViewID:      req.ViewID,
		Folder:      req.Folder,
		DashboardID: req.DashboardID,
		ProjectID:   req.ProjectID,
	})
	if err != nil {
		return nil, err
	}
	out := &ExportPreviewResult{}
	for _, p := range exported.Projects {
		out.Projects = append(out.Projects, p.Name)
	}
	for _, c := range exported.Connections {
		out.Connections = append(out.Connections, c.Alias)
	}
	for _, m := range exported.SemanticModels {
		out.SemanticModels = append(out.SemanticModels, m.Name)
	}
	for _, v := range exported.Views {
		out.Views = append(out.Views, v.Name)
	}
	for _, d := range exported.Dashboards {
		out.Dashboards = append(out.Dashboards, d.Name)
	}
	for _, a := range exported.Alerts {
		out.Alerts = append(out.Alerts, a.ID)
	}
	return out, nil
}

// ExportBundle writes the scope's slice to a path the OS dialog picks, so the
// webview never supplies one. The open document does not become the exported
// file: a scoped export is a delivery to another machine, not a relocation of
// the working file — and a sealed export must not become the document, or the
// next plain Save would refuse it with ErrSealedBlock.
func (s *QueryService) ExportBundle(req ExportRequest) (*ExportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.doc == nil {
		return nil, fmt.Errorf("no bundle is open — make one with NewUntitled or open a file first")
	}
	path, err := s.pickSavePath()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return &ExportResult{Cancelled: true}, nil
	}
	return s.exportLocked(req, path)
}

// exportLocked is the dialog-free core, so the tests can reach every decision
// without an OS window. Callers hold s.mu.
func (s *QueryService) exportLocked(req ExportRequest, path string) (*ExportResult, error) {
	if s.doc == nil {
		return nil, fmt.Errorf("no bundle is open — make one with NewUntitled or open a file first")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("bundle: %s: %w", path, err)
	}
	// Exporting over the open file would replace the full document with a
	// slice of it — the dialog is the only place that could pick that name, so
	// the guard lives here where the test can prove it.
	if abs == s.doc.path {
		return nil, fmt.Errorf("that is the open file itself — pick another name so the working file keeps its full contents")
	}

	exported, err := s.doc.bundle.Export(bundle.Scope{
		All:         req.All,
		ViewID:      req.ViewID,
		Folder:      req.Folder,
		DashboardID: req.DashboardID,
		ProjectID:   req.ProjectID,
	})
	if err != nil {
		return nil, err
	}

	result := &ExportResult{
		Projects:       len(exported.Projects),
		Connections:    len(exported.Connections),
		SemanticModels: len(exported.SemanticModels),
		Views:          len(exported.Views),
		Dashboards:     len(exported.Dashboards),
		Alerts:         len(exported.Alerts),
	}

	if !req.IncludeServerDetails {
		exported = exported.WithoutServerDetails()
	}

	if req.Passphrase != "" {
		plaintext, err := sealAddresses(exported)
		if err != nil {
			return nil, err
		}
		if plaintext == nil {
			return nil, fmt.Errorf("nothing to seal: this export carries no server address block — " +
				"either the scope has no server connection or the address toggle is off, so a passphrase would protect nothing")
		}
		if err := bundle.SaveWithSecrets(abs, exported, plaintext, req.Passphrase); err != nil {
			return nil, err
		}
		result.Sealed = true
	} else if err := bundle.Save(abs, exported); err != nil {
		return nil, err
	}

	result.Path = abs
	result.Name = filepath.Base(abs)
	return result, nil
}

// exportSealedBlock is the plaintext a passphrase protects on export: exactly
// the tier-2 material that would otherwise travel in the clear. M6's unseal
// reads the same shape back onto the connections.
type exportSealedBlock struct {
	Connections []exportSealedConnection `json:"connections"`
}

type exportSealedConnection struct {
	Alias   string             `json:"alias"`
	Driver  bundle.Driver      `json:"driver"`
	Address *bundle.Address    `json:"address,omitempty"`
	SSH     *bundle.SSHTarget  `json:"ssh,omitempty"`
}

// sealAddresses moves every Address/SSH block out of the bundle and returns
// them as the JSON plaintext for SaveWithSecrets. The bundle is the export's
// own copy (Export clones), so stripping here cannot touch the open document.
// nil, nil means there was nothing to move.
func sealAddresses(b *bundle.Bundle) ([]byte, error) {
	payload := exportSealedBlock{}
	for i := range b.Connections {
		c := &b.Connections[i]
		if c.Address == nil && c.SSH == nil {
			continue
		}
		payload.Connections = append(payload.Connections, exportSealedConnection{
			Alias: c.Alias, Driver: c.Driver, Address: c.Address, SSH: c.SSH,
		})
		c.Address = nil
		c.SSH = nil
	}
	if len(payload.Connections) == 0 {
		return nil, nil
	}
	return json.Marshal(&payload)
}

// ImportInspectResult is everything the import tab shows before Terapkan: the
// static pre-flight (alias conflicts, dedupe, paths) and the live merah/kuning
// schema lines from the scratch session.
type ImportInspectResult struct {
	Path string `json:"path"`
	Name string `json:"name"`

	Issues []bundle.ImportIssue `json:"issues"`
	Lines  []ImportCheckLine    `json:"lines"`
}

// ImportPick asks the OS which bundle the user means to import. An empty
// result with no error is a cancel: the dialog's "no" is not a failure.
func (s *QueryService) ImportPick() (string, error) {
	path, err := s.pickOpenPath()
	if err != nil {
		return "", err
	}
	return path, nil
}

// ImportInspect compares the incoming file against the open document twice —
// statically through bundle.Preflight and live through a scratch session that
// DESCRIBEs every View — and writes nothing. With no document open the static
// half runs against an empty bundle, which is the honest answer for a
// soon-to-be-created file.
func (s *QueryService) ImportInspect(path string) (*ImportInspectResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.importInspectLocked(path)
}

func (s *QueryService) importInspectLocked(path string) (*ImportInspectResult, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("bundle: %s: %w", path, err)
	}
	incoming, err := bundle.Open(abs)
	if err != nil {
		return nil, err
	}

	local := &bundle.Bundle{}
	if s.doc != nil {
		local = s.doc.bundle
	}
	report, err := local.Preflight(incoming, filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	lines, err := s.liveCheckBundle(incoming, abs)
	if err != nil {
		return nil, err
	}
	return &ImportInspectResult{Path: abs, Name: filepath.Base(abs), Issues: report.Issues, Lines: lines}, nil
}

// ImportApply merges the incoming file into the open document under the plan
// the dialog built, saves atomically, and reopens so the mounts reflect the
// merged sources. Merge's errors.Join refusal is the "batal = tidak ada yang
// berubah" half of FR-11: until every merah decision is in the plan, nothing
// is written.
func (s *QueryService) ImportApply(path string, plan bundle.ImportPlan) (*DocumentInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.importApplyLocked(path, plan)
}

func (s *QueryService) importApplyLocked(path string, plan bundle.ImportPlan) (*DocumentInfo, error) {
	if s.doc == nil {
		return nil, fmt.Errorf("no bundle is open — make one with NewUntitled or open a file first")
	}
	// The merge would carry the local Sealed block through, and then Save
	// refuses with ErrSealedBlock. Saying it here instead is the difference
	// between a door the user can walk through and an error code: unsealing a
	// bundle for merge is M6 work.
	if s.doc.bundle.Sealed != nil {
		return nil, fmt.Errorf("the open document is sealed (secrets.enc): importing into a sealed file is " +
			"not supported yet — save an unsealed copy first")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("bundle: %s: %w", path, err)
	}
	incoming, err := bundle.Open(abs)
	if err != nil {
		return nil, err
	}
	merged, err := s.doc.bundle.Merge(incoming, plan)
	if err != nil {
		return nil, err
	}
	if err := bundle.Save(s.doc.path, merged); err != nil {
		return nil, err
	}
	return s.openLocked(s.doc.path)
}
