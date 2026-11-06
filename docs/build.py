"""Builds the documentation site in every language, as FastAPI does: one Zensical build per language.

English, docs/en, is the source. A language lives in docs/<lang>/: lang.toml with its strings and
the pages translated so far. A page it lacks is taken from English with a notice, so every language
has every page. A translated page starts with front matter `source: <hash>`, the hash of the
English page it was translated from; `check` lists the pages whose English page changed since.

    python docs/build.py build          # the whole site into site/
    python docs/build.py build ru       # one language into .docs-build/ru/site, to check it
    python docs/build.py serve [lang]   # preview one language, English by default
    python docs/build.py check          # translations, highlights, and the Go blocks compile
    python docs/build.py tidy           # add the modules the Go blocks import to docs/gocheck

A program shown in variants, examples/<name>/<variant>/main.go, highlights with hl_lines the lines
that make it differ from the baseline variant ("without" or "ttl-only"; with two variants and no
baseline, from the other one). `check` compares those numbers with the programs.
"""

import difflib
import hashlib
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tomllib

ROOT = pathlib.Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs"
EN = DOCS / "en"
STAGE = ROOT / ".docs-build"
CONFIG = ROOT / "zensical.toml"
DESCRIPTION = 'site_description = "Caching for Go services that run as many processes"'
SITE_URL = 'site_url = "https://eugeneliukindev.github.io/gashe/"'


def languages() -> list[str]:
    """The languages of the selector in zensical.toml, English first."""
    config = tomllib.loads(CONFIG.read_text())
    alternate = config["project"].get("extra", {}).get("alternate", [])
    return [a["lang"] for a in alternate] or ["en"]


def source_hash(page: pathlib.Path) -> str:
    return hashlib.sha256(page.read_bytes()).hexdigest()[:12]


def pages(base: pathlib.Path) -> list[pathlib.Path]:
    return sorted(p.relative_to(base) for p in base.rglob("*.md"))


def with_notice(text: str, notice: str) -> str:
    """Puts the notice of an untranslated page under its first heading, or on top."""
    block = f'!!! info ""\n    {notice}\n'
    m = re.search(r"^# .*\n", text, re.M)
    if not m:
        return block + "\n" + text
    return text[: m.end()] + "\n" + block + text[m.end() :]


def stage(lang: str) -> pathlib.Path:
    """Writes the pages and the config of lang into .docs-build/<lang>/; returns the config."""
    src = DOCS / lang
    strings = tomllib.loads((src / "lang.toml").read_text())
    out = STAGE / lang
    shutil.rmtree(out, ignore_errors=True)
    shutil.copytree(EN, out / "docs")
    translated = set(pages(src))
    for page in pages(EN):
        if page not in translated:
            target = out / "docs" / page
            target.write_text(with_notice(target.read_text(), strings["untranslated"]))
    for f in src.rglob("*"):
        if f.is_file() and f.name != "lang.toml":
            dest = out / "docs" / f.relative_to(src)
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy(f, dest)

    config = CONFIG.read_text()
    replace = {
        'docs_dir = "docs/en"': 'docs_dir = "docs"',  # Zensical keeps both inside the config's folder
        'language = "en"': f'language = "{lang}"',
        SITE_URL: SITE_URL[:-1] + f'{lang}/"',  # canonical links, cards and llms.txt of the language
        'edit_uri = "edit/main/docs/en/"': f'edit_uri = "edit/main/docs/{lang}/"',
    }
    if "description" in strings:
        replace[DESCRIPTION] = f'site_description = "{strings["description"]}"'
    for title, translation in strings.get("nav", {}).items():
        replace[f'{{ "{title}" ='] = f'{{ "{translation}" ='
    for old, new in replace.items():
        if old not in config:
            sys.exit(f"docs/build.py: {old!r} is not in zensical.toml")
        config = config.replace(old, new)
    if "social_font" in strings:
        # The default font of the cards has no CJK characters.
        social = "[project.plugins.social]\n"
        config = config.replace(social, social + f'cards_layout_options = {{ font_family = "{strings["social_font"]}" }}\n')
    if "search_separator" in strings:
        # Languages written without spaces between words are searched per character.
        config += f"\n[project.plugins.search]\nseparator = '{strings['search_separator']}'\n"
    path = out / "zensical.toml"
    path.write_text(config)
    return path


def zensical(*args: str) -> None:
    subprocess.run(["zensical", *args], cwd=ROOT, check=True)


def build() -> None:
    shutil.rmtree(ROOT / "site", ignore_errors=True)
    zensical("build", "--strict", "-f", str(CONFIG))  # first: it empties site/
    for lang in languages()[1:]:
        config = stage(lang)
        zensical("build", "--strict", "-f", str(config))
        shutil.move(config.parent / "site", ROOT / "site" / lang)


def check() -> int:
    behind = 0
    for lang in languages()[1:]:
        src = DOCS / lang
        done = 0
        for page in pages(src):
            en = EN / page
            m = re.match(r"---\nsource: (\w+)\n", (src / page).read_text())
            if not en.exists():
                print(f"{lang}/{page}: no English page")
            elif not m:
                print(f"{lang}/{page}: no `source:` front matter, the English page is {source_hash(en)}")
            elif m.group(1) != source_hash(en):
                print(f"{lang}/{page}: English changed, translated from {m.group(1)}, now {source_hash(en)}")
            else:
                done += 1
                continue
            behind += 1
        print(f"{lang}: {done} of {len(pages(EN))} pages translated and current")
    return 1 if behind else 0


def changed_lines(lines: list[str], against: list[str]) -> list[int]:
    """The 1-based numbers of the lines of lines that against does not have. Spacing does not count,
    as gofmt aligns the fields around an added one, nor does the doc comment above `package`."""

    def norm(ls: list[str]) -> list[str]:
        return [" ".join(line.split()) for line in ls]

    matcher = difflib.SequenceMatcher(a=norm(against), b=norm(lines), autojunk=False)
    start = next((i for i, line in enumerate(lines) if line.startswith("package ")), 0)
    return [
        n + 1
        for tag, _, _, j1, j2 in matcher.get_opcodes()
        if tag in ("insert", "replace")
        for n in range(j1, j2)
        if n > start
    ]


BASELINES = ("without", "ttl-only")


def expected_highlight(program: pathlib.Path) -> list[int] | None:
    """The lines program must highlight, or None if it is not one of several variants."""
    example = program.parent.parent  # examples/<name> for a variant
    if example == ROOT / "examples":
        return None  # a program of its own, not a variant
    variants = sorted(p.parent for p in example.glob("*/main.go"))
    if len(variants) < 2:
        return None
    if program.parent.name in BASELINES:
        return []
    baseline = [v for v in variants if v.name in BASELINES]
    other = baseline or [v for v in variants if v != program.parent]
    read = lambda p: p.read_text().rstrip("\n").split("\n")  # noqa: E731
    return changed_lines(read(program), read(other[0] / "main.go"))


def check_highlights() -> int:
    """Lists the variants whose hl_lines do not match their programs."""
    # changed_lines itself: an added line and a changed one count, spacing and the doc comment not.
    assert changed_lines(["a", "b", "c"], ["a", "c"]) == [2]
    assert changed_lines(["a", "B", "c"], ["a", "b", "c"]) == [2]
    assert changed_lines(["// with", "package main", "x  := 1"], ["// without", "package main", "x := 1"]) == []
    fence = re.compile(r'^([ \t]*)```go(?: hl_lines="([\d ]*)")?\n\1--8<-- "(examples/[\w/-]+/main\.go)"', re.M)
    wrong = 0
    for page in pages(EN):
        for m in fence.finditer((EN / page).read_text()):
            want = expected_highlight(ROOT / m.group(3))
            have = [int(n) for n in (m.group(2) or "").split()]
            lines = (ROOT / m.group(3)).read_text().split("\n")
            if want is None:  # a program of its own: hl_lines is set by hand, so check it points at code
                for n in have:
                    if n > len(lines) or not lines[n - 1].strip() or lines[n - 1].strip().startswith("//"):
                        print(f"en/{page}: {m.group(3)}: hl_lines {n} is not a line of code")
                        wrong += 1
                continue
            if have != want:
                print(f'en/{page}: {m.group(3)}: hl_lines="{" ".join(map(str, have))}", '
                      f'want hl_lines="{" ".join(map(str, want))}"')
                wrong += 1
    return wrong


# Packages a fragment of the docs may use, by the name it uses them under.
IMPORTS = {
    "gashe": "github.com/eugeneliukindev/gashe",
    "memory": "github.com/eugeneliukindev/gashe/backend/memory",
    "nearcache": "github.com/eugeneliukindev/gashe/backend/nearcache",
    "gasheredis": "github.com/eugeneliukindev/gashe/backend/redis",
    "backendtest": "github.com/eugeneliukindev/gashe/backendtest",
    "httpcache": "github.com/eugeneliukindev/gashe/httpcache",
    "msgpack": "github.com/eugeneliukindev/gashe/codec/msgpack",
    "raw": "github.com/eugeneliukindev/gashe/codec/raw",
    "gzip": "github.com/eugeneliukindev/gashe/compress/gzip",
    "zstd": "github.com/eugeneliukindev/gashe/compress/zstd",
    "goredis": "github.com/redis/go-redis/v9",
    **{name: name for name in ("context", "errors", "fmt", "io", "log", "math", "os", "strconv", "strings",
                               "sync", "time", "slices", "maps", "testing")},
    "http": "net/http",
    "sql": "database/sql",
    "atomic": "sync/atomic",
}
# Variables fragments use without declaring them, as the pages do.
GIVEN = {
    "ctx": "var ctx context.Context\n",
    "c": "var c *gashe.Cache\n",
    "client": "var client *goredis.Client\n",
    "w": "var w http.ResponseWriter\n",
    "r": "var r *http.Request\n",
    "db": "var db *sql.DB\n",
}
# Errors a fragment may have: it uses names of the text around it and drops what it declares.
FORGIVEN = re.compile(
    r"declared and not used|imported and not used|undefined: [A-Za-z_]\w*$|is not used$"
    r"|too many return values"  # a fragment of a function that returns, put into main
)


def is_program(code: str) -> bool:
    return re.search(r"^package \w+", code, re.M) is not None


# Errors of a fragment that is not Go on its own, such as the fields of a struct literal.
SYNTAX = re.compile(r"^(expected |missing ',' |illegal |syntax error|unexpected )|^label \w+ ")


def fragment_program(code: str) -> str:
    """A Go file that holds code: a program as it is, declarations at package level, statements
    in a func, with the imports of the names it uses."""
    if is_program(code):
        return code
    # The names fragments take from the text around them, so the calls on them are checked too.
    given = {name: decl for name, decl in GIVEN.items() if re.search(rf"\b{name}\b", code)}
    names = code + "".join(given.values())
    used = [name for name in IMPORTS if re.search(rf"\b{name}\.", names)]
    imports = "\n".join(f'\t{name} "{IMPORTS[name]}"' for name in used)
    declarations = re.match(r"(func|type|var|const) ", code.lstrip()) is not None
    body = code if declarations else "func main() {\n" + code + "\n}"
    return f"package main\n\nimport (\n{imports}\n)\n\n" + "".join(given.values()) + f"\n{body}\n"


def check_code(tidy: bool = False) -> int:
    """Compiles every Go block of the English pages in the module docs/gocheck, so a block that
    uses API that does not exist fails. A block that includes a program of examples/ is built
    there instead, and one right after `<!-- not compiled: <why> -->` is left out, as one that shows
    two versions of a type. With tidy, it first adds the modules the blocks import to go.mod."""
    fence = re.compile(r"^([ \t]*)```go[^\n]*\n(.*?)^\1```", re.M | re.S)
    module = DOCS / "gocheck"
    work = module / "blocks"
    shutil.rmtree(work, ignore_errors=True)
    blocks = {}
    for page in pages(EN):
        text = (EN / page).read_text()
        for m in fence.finditer(text):
            ind = m.group(1)
            code = "\n".join(line[len(ind):] for line in m.group(2).rstrip("\n").split("\n"))
            if "--8<--" in code or re.search(r"<!-- not compiled: .+ -->\s*$", text[: m.start()]):
                continue  # a program of examples/, built there, or a block marked as not Go
            n = len(blocks)
            blocks[n] = (page, text[: m.start()].count("\n") + 1, is_program(code))
            (work / f"b{n}").mkdir(parents=True)
            src = fragment_program(code)
            if "func main()" not in src:
                src += "\nfunc main() {}\n"
            (work / f"b{n}" / "main.go").write_text(src)
    env = {**os.environ, "GOWORK": "off"}
    try:
        if tidy:
            subprocess.run(["go", "mod", "tidy"], cwd=module, env=env, check=True)
        # Build with -e: vet stops at the first type error of a package, and the build reports all.
        out = subprocess.run(["go", "build", "-gcflags=-e", *[f"./blocks/b{n}" for n in blocks]], cwd=module,
                             capture_output=True, text=True, env=env)
    finally:
        shutil.rmtree(work, ignore_errors=True)
    wrong, skipped = 0, set()
    for line in out.stderr.splitlines():
        m = re.match(r"(?:vet: )?(?:\./)?blocks/b(\d+)/main\.go:\d+:\d+: (.*)", line)
        if not m or FORGIVEN.search(m.group(2)):
            continue
        page, at, program = blocks[int(m.group(1))]
        if not program and SYNTAX.search(m.group(2)):
            skipped.add(int(m.group(1)))  # a part of a literal or a signature, not Go on its own
            continue
        print(f"en/{page}:{at}: {m.group(2)}")
        wrong += 1
    print(f"Go blocks: {len(blocks)} compiled, {len(skipped)} skipped as not Go on their own, {wrong} errors")
    return wrong


def main() -> int:
    cmd = sys.argv[1] if len(sys.argv) > 1 else "build"
    if cmd == "build" and len(sys.argv) > 2:
        zensical("build", "--strict", "-f", str(stage(sys.argv[2])))
    elif cmd == "build":
        build()
    elif cmd == "serve":
        lang = sys.argv[2] if len(sys.argv) > 2 else "en"
        # shortcut: a language other than English is served from a copy, so edits need a restart.
        zensical("serve", "-f", str(CONFIG if lang == "en" else stage(lang)))
    elif cmd == "tidy":
        return 1 if check_code(tidy=True) else 0
    elif cmd == "check":
        return 1 if check() + check_highlights() + check_code() else 0
    else:
        sys.exit(__doc__)
    return 0


if __name__ == "__main__":
    sys.exit(main())
