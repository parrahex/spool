#!/usr/bin/env python3
"""Auto-typing CLI demo for screen recordings.

Run this in a clean terminal window with a large font and minimal prompt:

    export PS1='$ '
    clear
    python3 scripts/record-cli-demo.py

The script builds binaries, starts a worker, types commands slowly, and shows
real output.
"""

import os
import random
import re
import subprocess
import sys
import time


ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def run(cmd, capture=True, timeout=30):
    result = subprocess.run(
        cmd,
        shell=True,
        cwd=ROOT,
        capture_output=capture,
        text=True,
        timeout=timeout,
    )
    if capture:
        return result.stdout.strip(), result.returncode
    return "", result.returncode


def clear():
    os.system("clear" if os.name != "nt" else "cls")


def typewrite(text, base_delay=0.04):
    """Type text with human-like irregular timing."""
    for i, char in enumerate(text):
        # Humans type in bursts: words a bit faster, punctuation slower.
        delay = base_delay * random.uniform(0.6, 1.8)
        if char in " .-/:":
            delay *= 1.5
        if i > 0 and text[i - 1] == " " and char.isalpha():
            delay *= 1.2
        sys.stdout.write(char)
        sys.stdout.flush()
        time.sleep(delay)


def type_command(cmd, base_delay=0.04):
    sys.stdout.write("$ ")
    sys.stdout.flush()
    time.sleep(0.4)
    typewrite(cmd, base_delay)
    time.sleep(0.6)
    sys.stdout.write("\n")
    sys.stdout.flush()


def pause(seconds=1.0):
    time.sleep(seconds)


def wait_for_status(job_id, target="completed", timeout=30):
    start = time.time()
    while time.time() - start < timeout:
        out, _ = run(f"./spool status {job_id}", timeout=120)
        if target in out:
            return out
        time.sleep(0.5)
    return out


def set_terminal_title(title):
    # macOS Terminal / iTerm2 window title
    sys.stdout.write(f"\033]0;{title}\007")
    sys.stdout.flush()


def main():
    set_terminal_title("zsh")

    bin_dir = "/tmp/spool-demo"
    os.makedirs(bin_dir, exist_ok=True)

    spool_bin = os.path.join(ROOT, "spool")
    worker_bin = os.path.join(bin_dir, "spool-worker")

    run(f'go build -o "{spool_bin}" ./cmd/cli')
    run(f'go build -o "{worker_bin}" ./cmd/worker')

    worker = subprocess.Popen(
        [worker_bin],
        cwd=ROOT,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    time.sleep(1)

    try:
        clear()
        pause(5.0)

        cmd1 = "./spool run --path test-artifacts --image python:3.11 python script.py"
        type_command(cmd1)
        out, _ = run(cmd1, timeout=120)
        print(out)
        pause(1.5)

        job_id = re.search(r"enqueued job: ([a-f0-9-]+)", out)
        if not job_id:
            print("# error: could not parse job id")
            sys.exit(1)
        job_id = job_id.group(1)

        pause(1.5)
        cmd2 = f"./spool status {job_id}"
        type_command(cmd2)
        status_out = wait_for_status(job_id, target="completed", timeout=30)
        print(status_out)
        pause(1)
    finally:
        worker.terminate()
        worker.wait()
        try:
            os.remove(spool_bin)
        except FileNotFoundError:
            pass


if __name__ == "__main__":
    main()
