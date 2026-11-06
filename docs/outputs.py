"""Writes the outputs the documentation shows into docs/snippets/, from real runs.

Every example, examples/<name>/main.go or a variant examples/<name>/<variant>/main.go, prints into
docs/snippets/examples/<name>[-<variant>].txt. The requests against examples/httpapi go into
docs/snippets/http/<case>.curl (the command) and .http (the response, its JSON formatted for
reading). examples/lockonmiss runs as three processes at once against a Redis in Docker on
localhost:6379. The Date header is replaced with a fixed one and the port shown is 8080, so a run
changes the files only when the behavior changes. Needs Go, curl and Docker.

    python docs/outputs.py
"""

import json
import pathlib
import re
import shutil
import socket
import subprocess
import sys
import time

ROOT = pathlib.Path(__file__).resolve().parent.parent
EXAMPLES = ROOT / "examples"
OUT = ROOT / "docs" / "snippets"
# nearcache and lockonmiss need Redis, and httpapi is a server: lockonmiss and httpapi run below.
SKIP = ("nearcache", "httpapi", "lockonmiss")
DATE = "Date: Fri, 06 Nov 2026 12:00:00 GMT"


def run_examples() -> None:
    (OUT / "examples").mkdir(parents=True, exist_ok=True)
    for main in sorted(EXAMPLES.rglob("main.go")):
        path = main.parent.relative_to(EXAMPLES).as_posix()
        name = path.replace("/", "-")  # lockonmiss/with -> lockonmiss-with
        if name.startswith(SKIP):
            continue
        out = subprocess.run(["go", "run", f"./{path}"], cwd=EXAMPLES, capture_output=True, text=True, check=True)
        (OUT / "examples" / f"{name}.txt").write_text(out.stdout)
        print(f"examples/{path}")


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("localhost", 0))
        return s.getsockname()[1]


def shell(arg: str) -> str:
    """Quotes arg for a shell when it needs it."""
    return arg if re.fullmatch(r"[\w./:=-]+", arg) else "'" + arg.replace("'", "'\\''") + "'"


def pretty(response: str) -> str:
    """Indents a JSON body for reading; the headers stay as they came."""
    head, _, body = response.partition("\n\n")
    if re.search(r"^Content-Type: application/json", head, re.M | re.I) and body.strip():
        body = json.dumps(json.loads(body), indent=2, ensure_ascii=False) + "\n"
    return f"{head}\n\n{body}".rstrip() + "\n"


def run_curl() -> None:
    port = free_port()
    binary = ROOT / ".docs-build" / "httpapi"
    binary.parent.mkdir(exist_ok=True)
    subprocess.run(["go", "build", "-o", str(binary), "./httpapi"], cwd=EXAMPLES, check=True)
    server = subprocess.Popen([str(binary)], env={"ADDR": f"localhost:{port}"}, stderr=subprocess.DEVNULL)
    try:
        for _ in range(100):
            try:
                socket.create_connection(("localhost", port), timeout=0.1).close()
                break
            except OSError:
                time.sleep(0.05)
        out = OUT / "http"
        shutil.rmtree(out, ignore_errors=True)
        out.mkdir(parents=True)

        def request(case: str, path: str, method: str = "GET", headers: tuple[str, ...] = ()) -> str:
            """Sends a request with curl; writes the command and the response."""
            url = f"localhost:8080{path}"
            curl = ["curl", "-i"] + (["-X", method] if method != "GET" else [])
            for h in headers:
                curl += ["-H", h]
            (out / f"{case}.curl").write_text(" ".join(shell(a) for a in [*curl, url]) + "\n")
            real = [*curl, url.replace("8080", str(port))]
            text = subprocess.run(real, capture_output=True, text=True, check=True).stdout
            text = re.sub(r"^Date: .*$", DATE, text.replace("\r\n", "\n"), flags=re.M)
            (out / f"{case}.http").write_text(pretty(text))
            print(f"http/{case}")
            return text

        first = request("miss", "/products")
        request("hit", "/products")
        etag = re.search(r"^Etag: (.*)$", first, re.M).group(1)
        request("not-modified", "/products", headers=(f"If-None-Match: {etag}",))
        request("query", "/products?utm_source=ads")
        request("vary", "/products", headers=("Accept-Language: ru",))
        request("cookie", "/products", headers=("Cookie: session=abc",))
        request("method", "/products", method="POST")

        request("report", "/report")
        time.sleep(1.5)  # past the TTL of a second: the handler runs again and fails
        request("stale", "/report")

        busy = subprocess.Popen(["curl", "-s", "-o", "/dev/null", f"localhost:{port}/export?id=1"])
        time.sleep(0.2)  # the first export takes the only render slot
        request("overloaded", "/export?id=2")
        busy.wait()

        for case in ("login-1", "login-2", "login-3"):
            request(case, "/login")
    finally:
        server.terminate()
        server.wait()


def build(path: str) -> pathlib.Path:
    binary = ROOT / ".docs-build" / path.replace("/", "-")
    binary.parent.mkdir(exist_ok=True)
    subprocess.run(["go", "build", "-o", str(binary), f"./{path}"], cwd=EXAMPLES, check=True)
    return binary


def docker(*args: str) -> str:
    return subprocess.run(["docker", *args], capture_output=True, text=True, check=True).stdout.strip()


def run_lockonmiss() -> None:
    """Runs examples/lockonmiss as three processes at once, without and with LockOnMiss, against a
    Redis in Docker on localhost:6379, the address the programs use."""
    with socket.socket() as s:
        if s.connect_ex(("127.0.0.1", 6379)) == 0:
            sys.exit("docs/outputs.py: localhost:6379 is taken; stop that Redis to run examples/lockonmiss")
    name = "gashe-docs-redis"
    subprocess.run(["docker", "rm", "-f", name], capture_output=True)
    docker("run", "-d", "--name", name, "-p", "127.0.0.1:6379:6379", "redis:7.0.0")
    try:
        for _ in range(100):
            if subprocess.run(["docker", "exec", name, "redis-cli", "ping"], capture_output=True, text=True).stdout.strip() == "PONG":
                break
            time.sleep(0.1)
        # Each process has its own memory, so only Redis is shared.
        for variant in ("without", "with"):
            program = build(f"lockonmiss/{variant}")
            docker("exec", name, "redis-cli", "flushall")
            procs = [subprocess.Popen([str(program)], env={"PATH": "/usr/bin:/bin"}, stdout=subprocess.PIPE, text=True)
                     for _ in range(3)]
            outs = sorted(p.communicate()[0].strip() for p in procs)
            lines = ["$ ./lockonmiss & ./lockonmiss & ./lockonmiss", *outs]
            (OUT / "examples" / f"lockonmiss-{variant}.txt").write_text("\n".join(lines) + "\n")
            print(f"examples/lockonmiss/{variant}")
    finally:
        subprocess.run(["docker", "rm", "-f", name], capture_output=True)


def main() -> int:
    run_examples()
    run_curl()
    run_lockonmiss()
    return 0


if __name__ == "__main__":
    sys.exit(main())
