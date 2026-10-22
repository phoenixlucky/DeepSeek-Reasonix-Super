import {
  BookOpen, Box, Braces, CalendarDays, CodeXml, Container, Database, File,
  FileArchive, FileAxis3d, FileChartColumn, FileCode, FileCog, FileDiff,
  FileDigit, FileImage, FileKey, FileMusic, FilePenLine, FileScan,
  FileSpreadsheet, FileTerminal, FileText, FileType, FileVideoCamera,
  GitBranch, Mail, NotebookPen, Package, Palette, PenTool, Presentation,
  Workflow, type LucideIcon,
} from "lucide-react";

const groups = [
  { kind: "markdown", icon: FilePenLine, extensions: "md markdown mdx mdown mkd" },
  { kind: "pdf", icon: FileScan, extensions: "pdf" },
  { kind: "spreadsheet", icon: FileSpreadsheet, extensions: "csv tsv xls xlsx xlsm xlsb ods numbers" },
  { kind: "document", icon: FileText, extensions: "doc docx docm dot dotx odt rtf pages" },
  { kind: "presentation", icon: Presentation, extensions: "ppt pptx pptm pps ppsx odp key" },
  { kind: "html", icon: CodeXml, extensions: "html htm xhtml" },
  { kind: "image", icon: FileImage, extensions: "png jpg jpeg gif webp avif bmp ico tif tiff heic heif raw dng" },
  { kind: "vector", icon: PenTool, extensions: "svg eps ai" },
  { kind: "design", icon: Palette, extensions: "psd psb sketch fig xd afdesign afphoto xcf" },
  { kind: "audio", icon: FileMusic, extensions: "mp3 wav flac aac m4a ogg opus aiff wma mid midi" },
  { kind: "video", icon: FileVideoCamera, extensions: "mp4 mov mkv webm avi m4v wmv flv mpeg mpg" },
  { kind: "archive", icon: FileArchive, extensions: "zip 7z rar tar gz tgz bz2 tbz2 xz txz zst cab" },
  { kind: "font", icon: FileType, extensions: "ttf otf woff woff2 eot" },
  { kind: "database", icon: Database, extensions: "sql db sqlite sqlite3 mdb accdb" },
  { kind: "data", icon: Braces, extensions: "json jsonc jsonl ndjson geojson" },
  { kind: "markup", icon: FileCode, extensions: "xml xsd xsl xslt plist rss atom" },
  { kind: "stylesheet", icon: Palette, extensions: "css scss sass less styl" },
  { kind: "config", icon: FileCog, extensions: "yaml yml toml ini conf cfg config properties env editorconfig" },
  { kind: "script", icon: FileTerminal, extensions: "sh bash zsh fish ps1 bat cmd" },
  { kind: "code", icon: FileCode, extensions: "js jsx mjs cjs ts tsx mts cts py pyw go rs java kt kts c h cpp hpp cc cxx cs swift rb php lua r jl dart vue svelte ex exs erl hs scala clj cljs elm sol zig nim pl pm groovy gradle cmake make proto graphql gql" },
  { kind: "notebook", icon: NotebookPen, extensions: "ipynb rmd qmd" },
  { kind: "text", icon: FileText, extensions: "txt text log rst adoc asciidoc tex bib" },
  { kind: "ebook", icon: BookOpen, extensions: "epub mobi azw azw3 fb2" },
  { kind: "package", icon: Package, extensions: "dmg pkg deb rpm apk aab ipa msi msix appx jar war vsix crx" },
  { kind: "binary", icon: FileDigit, extensions: "exe dll so dylib bin wasm o a lib class" },
  { kind: "model", icon: FileAxis3d, extensions: "obj stl fbx glb gltf blend 3ds step stp" },
  { kind: "cad", icon: Box, extensions: "dwg dxf" },
  { kind: "certificate", icon: FileKey, extensions: "pem crt cer p12 pfx pub asc gpg" },
  { kind: "mail", icon: Mail, extensions: "eml msg mbox" },
  { kind: "calendar", icon: CalendarDays, extensions: "ics ical" },
  { kind: "diff", icon: FileDiff, extensions: "diff patch" },
  { kind: "diagram", icon: Workflow, extensions: "drawio mmd mermaid puml plantuml dot" },
  { kind: "dataset", icon: FileChartColumn, extensions: "parquet arrow feather h5 hdf5 mat npy npz" },
] as const;

type FileAppearance = { kind: string; icon: LucideIcon };
const byExtension = new Map<string, FileAppearance>(
  groups.flatMap((group) => group.extensions.split(" ").map((extension) => [extension, group] as const)),
);
const byName: Record<string, FileAppearance> = {
  dockerfile: { kind: "container", icon: Container },
  containerfile: { kind: "container", icon: Container },
  makefile: { kind: "script", icon: FileTerminal },
  gnumakefile: { kind: "script", icon: FileTerminal },
  justfile: { kind: "script", icon: FileTerminal },
  ".gitignore": { kind: "git", icon: GitBranch },
  ".gitattributes": { kind: "git", icon: GitBranch },
  ".gitmodules": { kind: "git", icon: GitBranch },
  ".npmrc": { kind: "config", icon: FileCog },
  ".yarnrc": { kind: "config", icon: FileCog },
  ".bashrc": { kind: "script", icon: FileTerminal },
  ".zshrc": { kind: "script", icon: FileTerminal },
  license: { kind: "text", icon: FileText },
  readme: { kind: "text", icon: FileText },
};

export function fileAppearance(path: string): FileAppearance & { badge?: string } {
  const name = path.split(/[\\/]/).at(-1)!.toLowerCase();
  const extension = name.includes(".") ? name.split(".").at(-1)! : "";
  if (Object.hasOwn(byName, name)) return byName[name];
  if (name === ".env" || name.startsWith(".env.")) return { kind: "config", icon: FileCog };
  if (/^(dockerfile|containerfile)\./.test(name)) return { kind: "container", icon: Container };
  const appearance = byExtension.get(extension) ?? { kind: "unknown", icon: File };
  const badge = appearance.kind === "markdown" ? "MD"
    : ["pdf", "code", "data", "markup", "stylesheet"].includes(appearance.kind) ? extension.toUpperCase() : undefined;
  return { ...appearance, badge };
}

export function FileTypeIcon({ path }: { path: string }) {
  const { icon: Icon, kind, badge } = fileAppearance(path);
  return <span className="delivery-icon" data-file-kind={kind} aria-hidden="true">
    <Icon size={22} strokeWidth={1.5} />
    {badge && <span className="delivery-icon-badge">{badge}</span>}
  </span>;
}
