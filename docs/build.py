"""Builds the documentation site in every language, as FastAPI does: one Zensical build per language.

English, docs/en, is the source. A language lives in docs/<lang>/: lang.toml with its strings and
the pages translated so far. A page it lacks is taken from English with a notice, so every language
has every page. A translated page starts with front matter `source: <hash>`, the hash of the
English page it was translated from; `check` lists the pages whose English page changed since.

    python docs/build.py build          # the whole site into site/
    python docs/build.py build ru       # one language into .docs-build/ru/site, to check it
    python docs/build.py serve [lang]   # preview one language, English by default
    python docs/build.py check          # translations behind their English page
"""

import hashlib
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
    elif cmd == "check":
        return check()
    else:
        sys.exit(__doc__)
    return 0


if __name__ == "__main__":
    sys.exit(main())
