#!/usr/bin/env python3
import sys
import json
import hashlib
import argparse
from pathlib import Path

SALT = "leetgptsolver-v1"

def main():
    parser = argparse.ArgumentParser(description="Split dataset into train, validation, test, and unsolved.")
    parser.add_argument("input_file", help="Input JSONL file")
    parser.add_argument("--output-dir", default=".", help="Output directory for split files")
    args = parser.parse_args()

    input_path = Path(args.input_file)
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    files = {
        "train": open(output_dir / "train.jsonl", "w"),
        "validation": open(output_dir / "validation.jsonl", "w"),
        "test": open(output_dir / "test.jsonl", "w"),
        "unsolved": open(output_dir / "unsolved.jsonl", "w"),
    }

    try:
        with open(input_path, "r") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue

                try:
                    data = json.loads(line)
                except json.JSONDecodeError as e:
                    sys.exit(f"Error decoding JSON: {e}")

                split = None
                if data.get("solutions") is None:
                    split = "unsolved"
                else:
                    problem_id = data.get("id")
                    if problem_id is None:
                         sys.exit(f"Aborting: Found line without id: {line[:50]}...")


                    h = hashlib.md5(f"{problem_id}{SALT}".encode()).hexdigest()
                    val = int(h, 16)
                    normalized = val % 100

                    if normalized < 80:
                        split = "train"
                    elif normalized < 90:
                        split = "validation"
                    else:
                        split = "test"

                if split is None:
                    sys.exit(f"Aborting: Could not determine split for line: {line[:50]}...")

                files[split].write(line + "\n")
    finally:
        for f in files.values():
            f.close()

if __name__ == "__main__":
    main()
