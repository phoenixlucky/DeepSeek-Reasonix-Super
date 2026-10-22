import { useCallback, useEffect, useState } from "react";
import { t } from "../i18n";
import type { AgentPort, ShellOption, ShellSettings } from "../port/port";
import { reason, savedNotApplied } from "../i18n/kernel";
import { SaveNote } from "./SaveNote";

// The product name, not the executable: "powershell.exe" is what the file is
// called, "Windows PowerShell" is what the user installed.
const LABEL: Record<string, string> = {
  bash: "bash",
  "git-bash": "Git Bash",
  pwsh: "PowerShell 7",
  powershell: "Windows PowerShell",
};

// What changes for the person reading it. Only the 5.1 line is a warning: it is
// the one interpreter that refuses syntax the model writes by habit.
const NOTE: Record<string, string> = {
  pwsh: "支持 && 和 ||，但语法仍为 PowerShell，而非 bash",
  powershell: "不支持 && 和 ||，链式命令需拆分为两条",
};

const label = (o: ShellOption) => LABEL[o.name] ?? o.name;

// A typed-in path still has to say which family it belongs to, because that is
// what decides how the command is spelled — and the file name is the only clue
// on offer before it has been run.
const kindOf = (path: string) => (/pwsh/i.test(path) ? "pwsh" : /powershell/i.test(path) ? "powershell" : "bash");

export function Shell({ port, onChanged }: { port: AgentPort; onChanged?: () => void }) {
  const [s, setS] = useState<ShellSettings | null>(null);
  const [busy, setBusy] = useState("");
  const [note, setNote] = useState<{ text: string; unapplied: boolean } | null>(null);
  const [custom, setCustom] = useState("");
  const [launcher, setLauncher] = useState("");

  const load = useCallback(() => {
    port
      .shell()
      .then((v) => {
        setS(v);
        setCustom(v.path ?? "");
        setLauncher((v.args ?? []).join("\n"));
      })
      .catch(() => setS(null));
  }, [port]);

  useEffect(load, [load]);

  if (!s) return <div className="empty">{t("无法读取 shell 配置。")}</div>;

  const options = s.options ?? [];
  // Two bashes on one machine are two different programs, so picking one has to
  // pin its path. With only one, the name alone survives a PATH that moves.
  const pin = (o: ShellOption) => options.filter((x) => x.prefer === o.prefer).length > 1;
  const save = async (what: string, prefer: string, path: string, args?: string[]) => {
    setBusy(what);
    setNote(null);
    try {
      const next = await (args && args.length > 0 ? port.saveShell(prefer, path, args) : port.saveShell(prefer, path));
      setS(next);
      setCustom(next.path ?? "");
      setLauncher((next.args ?? []).join("\n"));
      onChanged?.();
    } catch (e) {
      setNote({ text: reason(e), unapplied: savedNotApplied(e) });
    } finally {
      setBusy("");
    }
  };

  const auto = s.prefer === "auto" && !s.path;
  const picked = (o: ShellOption) => !auto && s.effective.path === o.path;
  // A Windows host without bash is the case worth naming: the model's POSIX
  // habits are wrong there, and one install is what changes that.
  const noBash = s.platform === "windows" && !options.some((o) => o.prefer === "bash");
  // One argument per line: an argument may hold spaces (a distro name), and the
  // kernel runs the list as written, with no shell in between.
  const launcherArgs = launcher.split("\n").map((a) => a.trim()).filter((a) => a !== "");
  const fellBack = (s.args?.length ?? 0) > 0 && !s.argv;

  return (
    <div className="shell">
      <div className="kv">
        <span className="k">{t("当前生效")}</span>
        <span className="v">
          {label(s.effective)}
          {s.effective.version ? ` ${s.effective.version}` : ""} · {s.effective.path || "—"}
        </span>
      </div>

      {s.argv && (
        <div className="kv">
          <span className="k">{t("启动命令")}</span>
          <span className="v"><code>{s.argv.join(" ")} -c …</code></span>
        </div>
      )}
      {fellBack && <p className="note" data-tone="warn">{t("保存的启动参数现在无法运行，已回退到自动检测。")}</p>}

      {/* One shape for "pick one of a few", the same as every other such choice
          in this pane. The executable's path is too long for a segment, and it
          is not what you pick by anyway — it rides the 当前生效 row above and
          the note below, where it can be read rather than scanned. */}
      <div className="seg" data-text role="radiogroup" aria-label={t("命令执行程序")}>
        <button role="radio" data-action="shell.executor" aria-checked={auto} disabled={!!busy} onClick={() => void save("auto", "auto", "")}>
          {t("自动")}
        </button>
        {options.map((o) => (
          <button key={o.path} role="radio" data-action="shell.executor" aria-checked={picked(o)} disabled={!!busy}
            onClick={() => void save(o.path, o.prefer, pin(o) ? o.path : "")}>
            {label(o)}
            {o.version && <i className="ver">{o.version}</i>}
          </button>
        ))}
      </div>
      <p className="note">
        {auto ? (
          <>
            {t("自动查找，优先选择原生 bash。本机将选用")} {label(s.auto)}
          </>
        ) : (
          <>
            <code>{s.effective.path}</code>
            {NOTE[s.effective.name] && <> · {t(NOTE[s.effective.name])}</>}
          </>
        )}
      </p>

      {noBash && (
        <p className="note">
          {t("本机没有 bash，因此命令只能按 PowerShell 语法编写。安装 Git for Windows 后会增加 Git Bash 选项。要用 WSL，请在下方指定 wsl.exe 并填写启动参数；其中的路径是 Linux 写法。")}
        </p>
      )}

      <details className="pinpath">
        <summary>
          <span className="fold">{t("指定一个可执行文件")}</span>
        </summary>
        <p className="note">
          {t("自行编译的 bash、MSYS2，或安装在其他位置的 pwsh 均可填写于此。保存前会实际执行一条命令进行验证，无法运行则不会写入配置。")}
        </p>
        <div className="fields">
          <label className="grow full">
            <span>{t("可执行文件路径")}</span>
            <input
              value={custom}
              placeholder={s.auto.path}
              onChange={(e) => setCustom(e.target.value)}
            />
          </label>
        </div>
        {s.platform === "windows" && (
          <div className="fields">
            <label className="grow full">
              <span>{t("启动参数（可选，每行一项）")}</span>
              <textarea
                data-action="shell.custom-args"
                rows={4}
                value={launcher}
                placeholder={"-d\nUbuntu\n--\n/bin/bash"}
                onChange={(e) => setLauncher(e.target.value)}
              />
              <small className="note">{t("例如 wsl.exe 需要 -d <发行版> -- /bin/bash 才会进入 shell。参数按原样传给程序，不经过任何 shell；保存前会带着它们运行一次来验证。")}</small>
            </label>
          </div>
        )}
        <div className="acts">
          {s.path && (
            <button className="act" data-action="shell.custom-path" disabled={!!busy} onClick={() => void save("clear", s.prefer, "")}>
              {t("取消指定")}
            </button>
          )}
          <button className="act" data-action="shell.custom-path" data-primary disabled={!!busy || !custom.trim()}
            onClick={() => void save("custom", launcherArgs.length > 0 ? "bash" : kindOf(custom), custom.trim(), launcherArgs)}>
            {t(busy === "custom" ? "验证中…" : "保存该路径")}
          </button>
        </div>
      </details>

      <SaveNote note={note} />
    </div>
  );
}
