import { useId } from "react";
import { t } from "../i18n";
import { boardNameProblem } from "./boardname";

interface Props {
  checked: boolean;
  name: string;
  disabled?: boolean;
  onChecked: (on: boolean) => void;
  onName: (name: string) => void;
}

const WORDS = {
  short: "名字至少 2 个字符。",
  long: "名字最多 30 个字符。",
  chars: "名字只能用字母、数字、空格、下划线或连字符，不能含 @ 或句点。",
} as const;

// The choice that rides with linking: a public name on the contribution board,
// offered already ticked and applied only when the person confirms.
export function FeedbackBoardJoin({ checked, name, disabled, onChecked, onName }: Props) {
  const id = useId();
  const typed = name.trim();
  const problem = checked && typed ? boardNameProblem(typed) : null;
  return (
    <div className="fbk-board-join">
      <label className="fbk-board-join-pick">
        <input type="checkbox" data-action-change="feedback.board-join-choice" checked={checked} disabled={disabled} onChange={(e) => onChecked(e.target.checked)} />
        <span>{t("加入贡献榜")}</span>
      </label>
      <p className="fbk-hint">{t("榜是公开的：任何人都能看到下面这个名字和被采纳的条数，看不到邮箱、安装 ID 或路径。随时可以在贡献榜里退出。")}</p>
      {checked && (
        <>
          <input
            id={id}
            className="fbk-board-name-field"
            data-action-change="feedback.board-name"
            aria-label={t("榜上显示的名字")}
            aria-invalid={problem ? true : undefined}
            value={name}
            maxLength={40}
            disabled={disabled}
            placeholder={t("榜上显示的名字")}
            onChange={(e) => onName(e.target.value)}
          />
          {problem ? <p className="fbk-hint" role="alert" data-tone="error">{t(WORDS[problem])}</p>
            : typed ? <p className="fbk-hint" aria-live="polite">{t("榜上将显示为：{name}", { name: typed })}</p>
            : <p className="fbk-hint">{t("请填写一个名字，或取消勾选就不加入。")}</p>}
        </>
      )}
    </div>
  );
}
