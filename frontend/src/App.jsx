import { useEffect, useState } from "react";
import { QueryService } from "../bindings/muara.bi/desktop/index.js";
import { TruncateReason } from "../bindings/muara.bi/engine/duckdb/models.js";

const DEFAULT_SQL = `SELECT o.channel, COUNT(*) AS orders, SUM(o.total) AS revenue
FROM sample.public.orders AS o
GROUP BY 1 ORDER BY revenue DESC`;

// The SQLite fixture has no `public` schema, so the same catalogue name resolves
// differently — the query follows whichever source was actually attached.
const SQLITE_SQL = `SELECT o.channel, COUNT(*) AS orders, SUM(o.total) AS revenue
FROM sample.orders AS o
GROUP BY 1 ORDER BY revenue DESC`;

const STAT_LABELS = [
  ["duckdbVersion", "DuckDB"],
  ["maxMemory", "max_memory"],
  ["threads", "threads"],
  ["extensionDirectory", "extension_directory"],
  ["tempDirectory", "temp_directory"],
];

export default function App() {
  const [status, setStatus] = useState(null);
  const [result, setResult] = useState(null);
  const [message, setMessage] = useState("");
  const [isError, setIsError] = useState(false);
  const [sql, setSql] = useState(DEFAULT_SQL);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    QueryService.Status()
      .then((value) => {
        setStatus(value);
        setMessage("");
      })
      .catch((err) => report(err, true));
  }, []);

  function report(text, error) {
    setMessage(String(text));
    setIsError(!!error);
  }

  async function attach() {
    setBusy(true);
    try {
      await QueryService.AttachSample();
      report("sample ter-attach (READ_ONLY)");
    } catch (err) {
      report(err, true);
    } finally {
      setBusy(false);
    }
  }

  async function attachSQLite() {
    setBusy(true);
    try {
      await QueryService.AttachSampleSQLite();
      // Only rewrites the editor if the user never touched it, so a hand-typed
      // query is never thrown away by pressing a button.
      if (sql === DEFAULT_SQL) setSql(SQLITE_SQL);
      report("fixture sqlite ter-attach (READ_ONLY)");
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

  const note = result ? truncationNote(result) : null;

  return (
    <>
      <h1>M0 spike — Wails v3 × engine DuckDB</h1>

      <section className="card stats">
        {status ? (
          STAT_LABELS.map(([key, label]) => (
            <div className="stat" key={key}>
              <span>{label}</span>
              <strong>{status[key] || "(default duckdb)"}</strong>
            </div>
          ))
        ) : (
          <div className="note">{message || "Memuat setting engine…"}</div>
        )}
      </section>

      <section className="card">
        <div className="toolbar">
          <button onClick={attach} disabled={busy}>
            Attach Postgres sample
          </button>
          <button onClick={attachSQLite} disabled={busy}>
            Attach SQLite fixture
          </button>
          <button className="ghost" onClick={run} disabled={busy}>
            Jalankan query
          </button>
          {message ? (
            <span className={isError ? "err" : "note"}>{message}</span>
          ) : null}
        </div>

        <textarea
          value={sql}
          spellCheck={false}
          onChange={(event) => setSql(event.target.value)}
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
