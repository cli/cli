#!/usr/bin/env python3

import argparse
import glob
import json
import sys

parser = argparse.ArgumentParser()
parser.add_argument('globs', nargs='+', help='List of JSON file globs')
parser.add_argument('--write', action='store_true', help='Write out formatted files')
args = parser.parse_args()

needs_format = []

for pattern in args.globs:
    for cfg in glob.glob(pattern):
        with open(cfg, "r") as fr:
            existing = fr.read()
        j = json.loads(existing)
        new = json.dumps(j, indent="\t")
        new += "\n"
        if new != existing:
            if args.write:
                with open(cfg, "w") as fw:
                    fw.write(new)
            else:
                needs_format.append(cfg)

if len(needs_format) > 0:
    print("Files need reformatting:")
    for file in needs_format:
        print(f"\t{file}")
    print("Run ./test/format-configs.py --write 'test/config*/*.json'")
    sys.exit(1)
