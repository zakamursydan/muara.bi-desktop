import { useEffect, useState } from "react";
import { QueryService } from "../bindings/muara.bi/desktop/index.js";

const DEFAULT_SQL = `SELECT o.channel, COUNT(*) AS orders, SUM(o.total) AS revenue
FROM sample.public.orders AS o
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
          {result.truncated ? (
            <div className="badge">hasil terpotong oleh {result.truncated_by}</div>
          ) : null}
        </section>
      ) : null}
    </>
  );
}

function numeric(type) {
  return /INT|DECIMAL|FLOAT|DOUBLE|HUGE|UBIGINT/i.test(type);
}

function cell(value) {
  if (value === null || value === undefined) return "—";
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}
