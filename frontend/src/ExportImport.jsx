import { useEffect, useState } from "react";
import { QueryService } from "../bindings/muara.bi/desktop/index.js";

// Layar S5: Ekspor/Impor. Semua keputusan yang mengubah berkas (nama berkas
// ekspor, pilihan berkas impor) lewat dialog OS di sisi Go; webview hanya
// memegang nilai yang sudah dipilih. Terapkan impor menyala hanya ketika
// setiap masalah merah sudah diputuskan — itu yang membuat "batal = tidak ada
// yang berubah" benar di UI, bukan sekadar di Merge.

export default function ExportImport({ doc, onImported }) {
  const [tab, setTab] = useState("export");
  const [busy, setBusy] = useState(false);
  const [catalog, setCatalog] = useState(null);
  const [error, setError] = useState("");

  const [scopeKind, setScopeKind] = useState("all");
  const [scopeValue, setScopeValue] = useState("");
  const [includeAddress, setIncludeAddress] = useState(true);
  const [passphrase, setPassphrase] = useState("");
  const [preview, setPreview] = useState(null);
  const [done, setDone] = useState("");

  const [inspect, setInspect] = useState(null);
  const [forceReuse, setForceReuse] = useState({});
  const [skipViews, setSkipViews] = useState({});
  const [skipModels, setSkipModels] = useState({});
  const [overwriteViews, setOverwriteViews] = useState({});
  const [overwriteModels, setOverwriteModels] = useState({});
  const [newPaths, setNewPaths] = useState({});
  const [pins, setPins] = useState({});

  const open = !!doc?.open;

  useEffect(() => {
    if (!open) return;
    QueryService.Catalog()
      .then(setCatalog)
      .catch((err) => setError(String(err)));
  }, [doc?.path]);

  useEffect(() => {
    if (!open) return;
    if (scopeKind !== "all" && !scopeValue) {
      setPreview(null);
      return;
    }
    setError("");
    QueryService.ExportPreview(exportRequest())
      .then(setPreview)
      .catch((err) => setError(String(err)));
  }, [doc?.path, scopeKind, scopeValue]);

  function scopeOptions(kind) {
    if (!catalog) return [];
    if (kind === "folder") return (catalog.folders ?? []).map((f) => ({ id: f, name: f }));
    if (kind === "view") return catalog.views ?? [];
    if (kind === "dashboard") return catalog.dashboards ?? [];
    if (kind === "project") return catalog.projects ?? [];
    return [];
  }

  function changeKind(kind) {
    setScopeKind(kind);
    const options = scopeOptions(kind);
    setScopeValue(options.length ? options[0].id : "");
    setDone("");
  }

  function exportRequest() {
    return {
      all: scopeKind === "all",
      viewId: scopeKind === "view" ? scopeValue : "",
      folder: scopeKind === "folder" ? scopeValue : "",
      dashboardId: scopeKind === "dashboard" ? scopeValue : "",
      projectId: scopeKind === "project" ? scopeValue : "",
      includeServerDetails: includeAddress,
      passphrase: passphrase || "",
    };
  }

  async function doExport() {
    setBusy(true);
    setError("");
    setDone("");
    try {
      const res = await QueryService.ExportBundle(exportRequest());
      if (res.cancelled) {
        setDone("Dibatalkan — tidak ada yang ditulis.");
      } else {
        setDone(
          `Tersimpan: ${res.path} (${res.views} View, ${res.semanticModels} model, ` +
            `${res.connections} sumber${res.sealed ? " · alamat server tersegel passphrase" : ""})`,
        );
      }
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  }

  async function pickImport() {
    setBusy(true);
    setError("");
    try {
      const path = await QueryService.ImportPick();
      if (path) await inspectPath(path);
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  }

  async function inspectPath(path) {
    const res = await QueryService.ImportInspect(path);
    setInspect(res);
    setDone("");
    setForceReuse({});
    setSkipViews({});
    setSkipModels({});
    setOverwriteViews({});
    setOverwriteModels({});
    setNewPaths({});
    setPins({});
  }

  function togglePin(viewId, projectId) {
    const current = pins[viewId] ?? [];
    const next = current.includes(projectId)
      ? current.filter((id) => id !== projectId)
      : [...current, projectId];
    setPins({ ...pins, [viewId]: next });
  }

  function buildPlan() {
    return {
      skipViews,
      skipModels,
      overwriteViews,
      overwriteModels,
      forceReuseAliases: forceReuse,
      newPaths,
      pinViews: pins,
    };
  }

  function undecidedIssues() {
    if (!inspect) return [];
    const out = [];
    for (const issue of inspect.issues) {
      if (issue.level !== "merah") continue;
      if (issue.kind === "alias_conflict") {
        if (!forceReuse[issue.alias]) out.push(issue);
      } else if (issue.kind === "alias_missing") {
        const skipped = issue.viewId ? skipViews[issue.viewId] : skipModels[issue.modelId];
        if (!skipped) out.push(issue);
      } else if (issue.kind === "duplicate_diff") {
        const decided = issue.viewId
          ? skipViews[issue.viewId] || overwriteViews[issue.viewId]
          : skipModels[issue.modelId] || overwriteModels[issue.modelId];
        if (!decided) out.push(issue);
      } else {
        out.push(issue);
      }
    }
    return out;
  }

  async function doApply() {
    setBusy(true);
    setError("");
    try {
      const info = await QueryService.ImportApply(inspect.path, buildPlan());
      onImported(info);
      setInspect(null);
      setDone("");
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  }

  function annotatedIssues() {
    if (!inspect) return [];
    return inspect.issues.map((issue) => {
      const annotated = { ...issue };
      if (issue.kind === "alias_conflict") {
        annotated.decided = !!forceReuse[issue.alias];
      } else if (issue.kind === "alias_missing") {
        const key = issue.viewId ?? issue.modelId;
        annotated.decided = issue.viewId ? !!skipViews[key] : !!skipModels[key];
      } else if (issue.kind === "duplicate_diff") {
        const key = issue.viewId ?? issue.modelId;
        const overwritten = issue.viewId ? overwriteViews[key] : overwriteModels[key];
        const skipped = issue.viewId ? skipViews[key] : skipModels[key];
        annotated.decision = overwritten ? "overwrite" : skipped ? "skip" : "";
      } else if (issue.kind === "file_missing" || issue.kind === "file_folder") {
        annotated.newPath = newPaths[issue.alias];
      }
      return annotated;
    });
  }

  const undecided = undecidedIssues();
  const scopeSelect = scopeKind !== "all" ? scopeOptions(scopeKind) : [];
  const pinLines = (inspect?.lines ?? []).filter((l) => l.kind === "view");

  return (
    <section className="card">
      <div className="tabs">
        <button className={tab === "export" ? "tab active" : "tab"} onClick={() => setTab("export")}>
          Ekspor
        </button>
        <button className={tab === "import" ? "tab active" : "tab"} onClick={() => setTab("import")}>
          Impor
        </button>
      </div>

      {tab === "export" ? (
        <div>
          <div className="field">
            <span className="note">Lingkup:</span>
            {[
              ["all", "Semua"],
              ["view", "Satu View"],
              ["folder", "Folder"],
              ["dashboard", "Dashboard"],
              ["project", "Proyek"],
            ].map(([kind, label]) => (
              <label key={kind}>
                <input
                  type="radio"
                  name="scope"
                  checked={scopeKind === kind}
                  onChange={() => changeKind(kind)}
                />
                {label}
              </label>
            ))}
            {scopeKind !== "all" ? (
              <select
                value={scopeValue}
                onChange={(event) => {
                  setScopeValue(event.target.value);
                  setDone("");
                }}
                disabled={!scopeSelect.length}
              >
                {scopeSelect.map((option) => (
                  <option key={option.id} value={option.id}>
                    {option.name}
                  </option>
                ))}
              </select>
            ) : null}
          </div>

          {preview ? (
            <div className="preview">
              <h4>
                Yang akan ikut: {preview.views.length} View · {preview.semanticModels.length} model ·{" "}
                {preview.connections.length} sumber
              </h4>
              <ul>
                {preview.views.map((name) => (
                  <li key={`v/${name}`}>View: {name}</li>
                ))}
                {preview.semanticModels.map((name) => (
                  <li key={`m/${name}`}>Model: {name}</li>
                ))}
                {preview.connections.map((alias) => (
                  <li key={`c/${alias}`}>
                    <code>{alias}</code>
                  </li>
                ))}
                {preview.dashboards.map((name) => (
                  <li key={`d/${name}`}>Dashboard: {name}</li>
                ))}
                {preview.projects.map((name) => (
                  <li key={`p/${name}`}>Proyek: {name}</li>
                ))}
                {preview.alerts.map((id) => (
                  <li key={`a/${id}`}>Alert: {id}</li>
                ))}
              </ul>
            </div>
          ) : null}

          <div className="field">
            <label>
              <input
                type="checkbox"
                checked={includeAddress}
                onChange={(event) => {
                  setIncludeAddress(event.target.checked);
                  if (!event.target.checked) setPassphrase("");
                }}
              />
              Sertakan alamat server (tier 2)
            </label>
            <label>
              Passphrase (opsional):
              <input
                type="password"
                value={passphrase}
                onChange={(event) => setPassphrase(event.target.value)}
                disabled={!includeAddress}
                placeholder="kosongkan = alamat ikut apa adanya"
              />
            </label>
          </div>
          <div className="hint">
            {includeAddress
              ? "Alamat server ikut dalam JSON berkas. Dengan passphrase, alamat hanya masuk lewat secrets.enc."
              : "Alamat server tidak ikut: penerima mengisi sendiri lewat Profil mesinnya."}
          </div>

          <div className="toolbar" style={{ marginTop: 10 }}>
            <button
              onClick={doExport}
              disabled={busy || !open || (scopeKind !== "all" && !scopeValue)}
            >
              Ekspor…
            </button>
            <span className="hint">Berkas yang sedang terbuka tidak berubah — ekspor menulis salinan.</span>
          </div>

          {done ? <div className="note" style={{ marginTop: 8 }}>{done}</div> : null}
        </div>
      ) : (
        <div>
          {!inspect ? (
            <>
              <p className="note">
                Impor menggabungkan isi satu berkas .muara ke dokumen yang terbuka. Tidak ada yang
                ditulis sebelum Terapkan, dan keputusan merah harus dijawab dulu.
              </p>
              <button onClick={pickImport} disabled={busy}>
                Pilih berkas…
              </button>
            </>
          ) : (
            <>
              <div className="note" style={{ marginBottom: 8 }}>
                Berkas: <strong>{inspect.name}</strong> · {inspect.issues.length} temuan statis ·{" "}
                {inspect.lines.length} baris cek skema
              </div>

              {inspect.issues.length ? (
                <div>
                  <h4>Keputusan</h4>
                  {annotatedIssues().map((issue, index) => (
                    <div key={`${issue.kind}/${issue.alias ?? issue.viewId ?? issue.modelId ?? index}`} className={`issue ${issue.level}`}>
                      <span className={`chip level-${issue.level}`}>{issue.level}</span>{" "}
                      <strong>{issue.title}</strong>
                      {issue.detail ? <div className="detail">{issue.detail}</div> : null}
                      <Decision
                        issue={issue}
                        setForceReuse={setForceReuse}
                        setSkipViews={setSkipViews}
                        setSkipModels={setSkipModels}
                        setOverwriteViews={setOverwriteViews}
                        setOverwriteModels={setOverwriteModels}
                        setNewPaths={setNewPaths}
                      />
                    </div>
                  ))}
                </div>
              ) : (
                <div className="note">Tidak ada temuan statis.</div>
              )}

              {inspect.lines.length ? (
                <div>
                  <h4>Cek skema di mesin ini</h4>
                  {inspect.lines.map((line) => (
                    <div key={`${line.kind}/${line.name}`} className="listRow">
                      <span className={`chip level-${line.level}`}>{line.level}</span>
                      <strong>{line.name}</strong>
                      <span className="note">{line.kind === "view" ? "View" : "Model"}</span>
                      {line.reason ? <span className="detail">{line.reason}</span> : null}
                    </div>
                  ))}
                </div>
              ) : null}

              {pinLines.length && (catalog?.projects ?? []).length ? (
                <div>
                  <h4>Pin ke proyek dokumen ini</h4>
                  {pinLines.map((line) => (
                    <div key={`pin/${line.id}`} className="field">
                      <span>{line.name}:</span>
                      {(catalog.projects ?? []).map((project) => (
                        <label key={project.id}>
                          <input
                            type="checkbox"
                            checked={(pins[line.id] ?? []).includes(project.id)}
                            onChange={() => togglePin(line.id, project.id)}
                          />
                          {project.name}
                        </label>
                      ))}
                    </div>
                  ))}
                </div>
              ) : null}

              <div className="toolbar" style={{ marginTop: 10 }}>
                <button
                  onClick={doApply}
                  disabled={busy || !open || undecided.length > 0}
                  title={
                    undecided.length
                      ? `${undecided.length} keputusan merah belum dijawab`
                      : "Terapkan impor"
                  }
                >
                  Terapkan
                </button>
                <button className="ghost" onClick={() => setInspect(null)} disabled={busy}>
                  Batal
                </button>
                {undecided.length ? (
                  <span className="hint">{undecided.length} keputusan merah belum dijawab.</span>
                ) : null}
              </div>
            </>
          )}
        </div>
      )}

      {error ? <div className="err" style={{ marginTop: 8 }}>{error}</div> : null}
    </section>
  );
}

function Decision({
  issue,
  setForceReuse,
  setSkipViews,
  setSkipModels,
  setOverwriteViews,
  setOverwriteModels,
  setNewPaths,
}) {
  if (issue.kind === "alias_conflict") {
    return (
      <div className="decision">
        <label>
          <input
            type="checkbox"
            checked={!!issue.decided}
            onChange={(event) => setForceReuse((map) => ({ ...map, [issue.alias]: event.target.checked }))}
          />
          Pakai yang ada — alias tetap milik dokumen ini
        </label>
      </div>
    );
  }
  if (issue.kind === "alias_missing") {
    const setMap = issue.viewId ? setSkipViews : setSkipModels;
    const key = issue.viewId ?? issue.modelId;
    return (
      <div className="decision">
        <label>
          <input
            type="checkbox"
            checked={!!issue.decided}
            onChange={(event) => setMap((map) => ({ ...map, [key]: event.target.checked }))}
          />
          Lewati {issue.viewId ? "View ini" : "model ini"}
        </label>
      </div>
    );
  }
  if (issue.kind === "duplicate_diff") {
    const key = issue.viewId ?? issue.modelId;
    const setSkip = issue.viewId ? setSkipViews : setSkipModels;
    const setOverwrite = issue.viewId ? setOverwriteViews : setOverwriteModels;
    const choose = (value) => {
      setSkip((map) => ({ ...map, [key]: value === "skip" }));
      setOverwrite((map) => ({ ...map, [key]: value === "overwrite" }));
    };
    return (
      <div className="decision">
        <label>
          <input type="radio" name={`dup/${key}`} checked={issue.decision === "skip"} onChange={() => choose("skip")} />
          Lewati
        </label>
        <label>
          <input type="radio" name={`dup/${key}`} checked={issue.decision === "overwrite"} onChange={() => choose("overwrite")} />
          Timpa
        </label>
        {!issue.decision ? <span className="hint">belum diputuskan — Terapkan tetap terkunci</span> : null}
      </div>
    );
  }
  if (issue.kind === "file_missing" || issue.kind === "file_folder") {
    return (
      <div className="decision">
        <label>
          Tunjuk ulang:
          <input
            type="text"
            placeholder={issue.path}
            value={issue.newPath ?? ""}
            onChange={(event) => setNewPaths((map) => ({ ...map, [issue.alias]: event.target.value }))}
          />
        </label>
        <span className="hint">kosongkan = biarkan seperti tertulis di berkas, mount akan melaporkannya</span>
      </div>
    );
  }
  return null;
}
