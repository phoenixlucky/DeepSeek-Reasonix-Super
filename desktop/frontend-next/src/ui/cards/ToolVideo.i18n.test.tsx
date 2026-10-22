// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { boot, STORAGE } from "../../i18n";
import { ToolCard } from "./ToolCard";

afterEach(() => {
  cleanup();
  localStorage.setItem(STORAGE, "zh");
  boot();
});

const RAW = "read_video needs ffmpeg and ffprobe on this machine's PATH (refusal: video.ffmpeg_missing)";

describe.each([
  ["zh", "video.ffmpeg_missing", "读视频需要本机装有 ffmpeg（含 ffprobe）并能在系统路径里找到。装好后再试一次。"],
  ["en", "video.ffmpeg_missing", "Reading a video needs ffmpeg (with ffprobe) installed and on the system path. Install it and try again."],
  ["zh", "video.unsupported_container", "不认识这种视频格式。支持 MP4/MOV、MKV/WebM、AVI、FLV、ASF、MPEG 和 Ogg 这些常见封装。"],
  ["en", "video.too_large", "The video file is larger than the limit for reading."],
])("video refusals in %s", (lang, code, expected) => {
  it(`says ${code} in the reader's language, once, without the raw echo`, () => {
    localStorage.setItem(STORAGE, lang); boot();
    render(<ToolCard tool={{ id: "v", name: "read_video", args: '{"path":"clip.mp4"}', err: RAW, output: RAW, refusalCode: code, readOnly: true }} running={false} />);
    expect(screen.queryAllByText(expected)).toHaveLength(1);
    expect(screen.queryByText(RAW)).toBeNull();
  });
});

it("keeps the host's own detail for a decode failure, which has no wording of its own", () => {
  localStorage.setItem(STORAGE, "en"); boot();
  const detail = "ffmpeg could not read the frame at 3.0s: exit status 1 (refusal: video.decode_failed)";
  render(<ToolCard tool={{ id: "v", name: "read_video", args: '{"path":"clip.mp4"}', err: detail, output: detail, refusalCode: "video.decode_failed", readOnly: true }} running={false} />);
  expect(screen.queryAllByText(detail)).toHaveLength(1);
});
