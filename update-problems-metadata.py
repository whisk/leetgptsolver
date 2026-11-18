#!/usr/bin/env python3
import argparse
import json
import random
import subprocess
import sys
from pathlib import Path


def positive_int(value: str) -> int:
    try:
        parsed = int(value)
    except ValueError as e:
        raise argparse.ArgumentTypeError(f"count must be a positive integer, got '{value}'") from e
    if parsed <= 0:
        raise argparse.ArgumentTypeError(f"count must be a positive integer, got '{value}'")
    return parsed


def select_latest(files: list[Path], count: int) -> list[str]:
    ranked = []
    for path in files:
        with open(path, "r") as f:
            data = json.load(f)
        question = ((data.get("Question") or {}).get("Data") or {}).get("Question") or {}
        frontend_id = int(question["questionFrontendId"])
        ranked.append((frontend_id, path.name, str(path)))
    ranked.sort(key=lambda item: (item[0], item[1]))
    return [item[2] for item in ranked[-count:]]


def select_random(files: list[Path], count: int) -> list[str]:
    sample_size = min(count, len(files))
    return [str(p) for p in random.sample(files, sample_size)]


def main():
    parser = argparse.ArgumentParser(
        description="Update question metadata and stats while keeping existing solutions and submissions intact."
    )
    parser.add_argument(
        "-n",
        "--count",
        type=positive_int,
        default=100,
        help="Number of problems to update (default: 100)",
    )
    parser.add_argument(
        "-r",
        "--random",
        action="store_true",
        help="Select COUNT random problems instead of the latest COUNT by frontend ID",
    )
    parser.add_argument(
        "--problems-dir",
        default="problems",
        help="Directory containing problem JSON files (default: problems)",
    )
    args = parser.parse_args()

    problems_dir = Path(args.problems_dir)
    if not problems_dir.is_dir():
        sys.exit(f"Error: {problems_dir} is not a directory")

    files = sorted(problems_dir.glob("*.json"))
    if not files:
        sys.exit(f"Error: No json files found in {problems_dir}")

    selected = (
        select_random(files, args.count)
        if args.random
        else select_latest(files, args.count)
    )

    cmd = [
        "go",
        "run",
        ".",
        "download",
        "-D",
        str(problems_dir),
        "-c",
        "all",
        "-v",
        "-u",
        "--detect_approx_creation_date=false",
        "-",
    ]
    try:
        subprocess.run(
            cmd,
            input="\n".join(selected),
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        sys.exit(e.returncode)
    except Exception as e:
        sys.exit(f"Execution failed: {e}")


if __name__ == "__main__":
    main()
