package main

import (
	"fmt"
	"path/filepath"
)

// muaraFilter is the one file type the pickers show. A bundle is a zip, so the
// OS would otherwise offer every archive on the disk and the user would have to
// guess which one is theirs.
const muaraFilter = "*.muara"

// OpenWithDialog asks the OS which file the user means and then opens it.
//
// The dialog is Wails' own, reached from the service rather than from the
// webview: a path typed into a webview is a path the app would open without the
// user having seen it, and BRD §6 keeps file paths out of the UI's hands the same
// way it keeps credentials out of them.
func (s *QueryService) OpenWithDialog() (*DocumentInfo, error) {
	path, err := s.pickOpenPath()
	if err != nil {
		return nil, err
	}
	if path == "" {
		// Cancel is an answer, not a failure: the document the user was working on
		// stays exactly as it was.
		return s.Current()
	}
	return s.Open(path)
}

// SaveAsWithDialog is Export in BRD §5.1's sense — "Export = Save As" — with the
// folder chosen by the OS instead of typed.
func (s *QueryService) SaveAsWithDialog() (*DocumentInfo, error) {
	path, err := s.pickSavePath()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return s.Current()
	}
	return s.SaveAs(path)
}

func (s *QueryService) pickOpenPath() (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("no window is open to ask for a file")
	}
	dialog := s.app.Dialog.OpenFile().
		SetTitle("Buka berkas muara.bi").
		AddFilter("muara.bi bundle", muaraFilter).
		CanChooseDirectories(false)
	if s.doc != nil {
		dialog = dialog.SetDirectory(filepath.Dir(s.doc.path))
	}
	return dialog.PromptForSingleSelection()
}

func (s *QueryService) pickSavePath() (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("no window is open to ask for a file")
	}
	name := "Untitled.muara"
	if s.doc != nil {
		name = filepath.Base(s.doc.path)
	}
	dialog := s.app.Dialog.SaveFile().
		SetMessage("Simpan sebagai").
		SetFilename(name).
		AddFilter("muara.bi bundle", muaraFilter)
	if s.doc != nil {
		dialog = dialog.SetDirectory(filepath.Dir(s.doc.path))
	}
	return dialog.PromptForSingleSelection()
}
