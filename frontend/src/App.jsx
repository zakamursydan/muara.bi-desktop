import { useEffect, useState } from "react";
import { QueryService } from "../bindings/muara.bi/desktop/index.js";
import { TruncateReason } from "../bindings/muara.bi/engine/duckdb/models.js";

// With no document open nothing is mounted, so the starting query asks the engine
// about itself instead of naming a source that does not exist yet.
const START_SQL = "SELECT 1 AS one";

export default function App() {
  const [status, setStatus] = useState(null);
  const [doc, setDoc] = useState(null);
  const [result, setResult] = useState(null);
  const [message, setMessage] = useState("");
  const [isError, setIsError] = useState(false);
  const [sql, setSql] = useState(START_SQL);
  // The editor is only rewritten when the user has not typed in it: opening a
  // file must never throw away the query someone is already writing.
  const [edited, setEdited] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    Promise.all([QueryService.Status(), QueryService.Current()])
      .then(([engine, current]) => {
        setStatus(engine);
        setDoc(current);
        setMessage("");
      })
      .catch((err) => report(err, true));
  }, []);

  function report(text, error) {
    setMessage(String(text));
    setIsError(!!error);
  }

  async function call(action, done) {
    setBusy(true);
    try {
      const current = await action();
      setDoc(current);
      if (current?.open && !edited) {
        const suggested = suggestFor(current);
        if (suggested) {
          setSql(suggested);
          setEdited(false);
        }
      }
      report(done);
    } catch (err) {
      report(err, true);
    } finally {
      setBusy(false);
    }
  }

  async function run() {
    setBusy(true);
    try {
      setResult(await QueryService.Run(sql));
      report("");
    } catch (err) {
      report(err, true);
    } finally {
      setBusy(false);
    }
  }

  const open = !!doc?.open;
  const mounted = (doc?.mounts ?? []).filter((m) => m.mounted).length;
  const note = result ? truncationNote(result) : null;

  return (
    <>
      <header className="titlebar">
        <strong>muara.bi</strong>
        <span className="note">
          {status
            ? `DuckDB ${status.duckdbVersion} · ${
                status.maxMemory || "(default duckdb)"
              } · ${status.threads} thread`
            : "Memuat setting engine…"}
        </span>
      </header>

      <section className="card">
        <div className="toolbar">
          <button
            className="ghost"
            onClick={() => call(QueryService.NewUntitled, "Bundle baru dibuat")}
            disabled={busy}
          >
            Mulai kosong
          </button>
          <button
            className="ghost"
            onClick={() => call(QueryService.OpenWithDialog, "")}
            disabled={busy}
          >
            Buka…
          </button>
          <button onClick={() => call(QueryService.Save, "Tersimpan")} disabled={busy || !open}>
            Simpan
          </button>
          <button
            className="ghost"
            onClick={() => call(QueryService.SaveAsWithDialog, "")}
            disabled={busy || !open}
          >
            Simpan sebagai…
          </button>
          <span className={doc?.dirty ? "fileName dirty" : "fileName"}>
            {open ? `${doc.dirty ? "● " : ""}${doc.name}` : "Tidak ada berkas terbuka"}
          </span>
        </div>

        <div className="note docMeta">
          {open ? (
            <>
              {doc.folder} · {doc.views} View · {doc.semanticModels} model semantik ·{" "}
              {doc.connections} sumber{doc.dirty ? " · belum disimpan" : ""}
            </>
          ) : (
            "Buat bundle baru atau buka berkas .muara yang sudah ada."
          )}
        </div>

        {doc?.profileError ? (
          <div className="err">Profil mesin ini tidak terpakai: {doc.profileError}</div>
        ) : null}

        {open && doc.mounts?.length ? (
          <details className="sources">
            <summary>
              Sumber ({mounted} ter-mount dari {doc.mounts.length})
            </summary>
            <ul>
              {doc.mounts.map((m, index) => (
                <li key={`${m.kind}/${m.name}/${index}`}>
                  <span className={m.mounted ? "ok" : "bad"}>{m.mounted ? "✓" : "✗"}</span>
                  <code>{m.name}</code>
                  <span className="note">{kindLabel(m)}</span>
                  {m.target ? <span className="target">{m.target}</span> : null}
                  {!m.mounted && m.reason ? <div className="reason">{m.reason}</div> : null}
                </li>
              ))}
            </ul>
          </details>
        ) : null}

        {message ? <div className={isError ? "err" : "note"}>{message}</div> : null}
      </section>

      <section className="card">
        <div className="toolbar">
          <button className="ghost" onClick={run} disabled={busy}>
            Jalankan query
          </button>
        </div>

        <textarea
          value={sql}
          spellCheck={false}
          onChange={(event) => {
            setSql(event.target.value);
            setEdited(true);
          }}
        />
      </section>

      {result ? (
        <section className="card">
          <div className="tableWrap">
            <table>
              <thead>
                <tr>
                  {result.columns.map((col) => (
                    <th key={col.name} className={numeric(col.type) ? "num" : undefined}>
                      {col.name}
                      <span className="note"> · {col.type}</span>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {result.rows.map((row, index) => (
                  <tr key={index}>
                    {result.columns.map((col) => (
                      <td key={col.name} className={numeric(col.type) ? "num" : undefined}>
                        {cell(row[col.name])}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="note">
            {result.row_count} baris · {result.duration_ms} ms · cap {result.row_limit}
          </div>
          {note ? <div className="badge">{note}</div> : null}
        </section>
      ) : null}
    </>
  );
}

// The first thing worth running after a bundle opens is the SQL the bundle saved
// itself, then a plain file source. An attached SQLite is a catalogue rather than
// a relation, so naming its alias alone would be a query that fails.
function suggestFor(doc) {
  const readable = (doc.mounts ?? []).find(
    (m) =>
      m.mounted &&
      (m.kind === "semantic_model" || m.driver === "file") &&
      plainName(m.name),
  );
  return readable ? `SELECT * FROM ${readable.name} LIMIT 20` : null;
}

function plainName(name) {
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(name ?? "");
}

function kindLabel(m) {
  if (m.kind === "semantic_model") return "model semantik";
  return m.driver || m.kind;
}

// The engine decides whether an answer is complete; the grid only says it here.
function truncationNote(result) {
  if (!result?.truncated) return null;

  const sources = (result.capped_sources ?? []).filter(Boolean);
  if (result.truncated_by === TruncateReason.TruncateSourceCap) {
    return `sebagian sumber hanya terbaca sampai batasnya${
      sources.length ? `: ${sources.join(", ")}` : ""
    }`;
  }
  return (
    `baris dipotong ke ${result.row_limit} pertama` +
    (sources.length
      ? ` · sumber ${sources.join(", ")} juga hanya terbaca sebagian`
      : "")
  );
}

function numeric(type) {
  return /INT|DECIMAL|FLOAT|DOUBLE|HUGE|UBIGINT/i.test(type);
}

function cell(value) {
  if (value === null || value === undefined) return "—";
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}
