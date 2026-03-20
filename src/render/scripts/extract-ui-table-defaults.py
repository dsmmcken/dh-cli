#!/usr/bin/env python3
"""
Extract default prop values from deephaven.ui.components.table.UITable.__init__.

Parses the Python AST to find non-None default values, converts snake_case
to camelCase (matching DH's to_react_prop_case), and writes them as JSON.

Usage:
    python3 scripts/extract-ui-table-defaults.py > src/ui-table-defaults.json

    # Or auto-find the DH venv:
    scripts/extract-ui-table-defaults.py
"""
import ast
import json
import sys
import glob
import os
import re


def to_camel_case(snake: str) -> str:
    """Convert snake_case to camelCase, matching DH's to_react_prop_case."""
    # Strip trailing underscore (e.g., format_ -> format)
    if snake.endswith("_"):
        snake = snake[:-1]
    parts = snake.split("_")
    return parts[0] + "".join(p.capitalize() for p in parts[1:])


def extract_defaults(source: str) -> dict:
    """Parse UITable.__init__ signature and extract non-None defaults."""
    tree = ast.parse(source)

    for node in ast.walk(tree):
        if not isinstance(node, ast.ClassDef) or node.name not in ("table", "UITable"):
            continue
        for item in node.body:
            if not isinstance(item, ast.FunctionDef) or item.name != "__init__":
                continue

            defaults = {}
            args = item.args
            # Match defaults to parameter names (defaults align to the end of the arg list)
            all_params = args.args + args.kwonlyargs
            all_defaults = (
                [None] * (len(args.args) - len(args.defaults))
                + args.defaults
                + args.kw_defaults
            )

            for param, default in zip(all_params, all_defaults):
                name = param.arg
                if name == "self" or name == "table":
                    continue
                if default is None:
                    continue
                # ast.Constant covers None, True, False, numbers, strings
                if isinstance(default, ast.Constant):
                    if default.value is None:
                        continue  # None = no default to show
                    defaults[to_camel_case(name)] = default.value

            return defaults

    raise ValueError("Could not find UITable.__init__ in source")


def find_table_py() -> str:
    """Find the DH table.py source file in the installed venv."""
    patterns = [
        os.path.expanduser("~/.dh/versions/*/.venv/lib/*/site-packages/deephaven/ui/components/table.py"),
        os.path.expanduser("~/.dh/*/.venv/lib/*/site-packages/deephaven/ui/components/table.py"),
    ]
    for pattern in patterns:
        matches = sorted(glob.glob(pattern))
        if matches:
            return matches[-1]  # newest version
    return None


def main():
    if len(sys.argv) > 1:
        path = sys.argv[1]
    else:
        path = find_table_py()
        if not path:
            print("Could not find deephaven/ui/components/table.py", file=sys.stderr)
            print("Usage: extract-ui-table-defaults.py [path/to/table.py]", file=sys.stderr)
            sys.exit(1)
        print(f"Found: {path}", file=sys.stderr)

    source = open(path).read()
    defaults = extract_defaults(source)

    out_path = os.path.join(os.path.dirname(__file__), "..", "src", "ui-table-defaults.json")
    with open(out_path, "w") as f:
        json.dump(defaults, f, indent=2)
        f.write("\n")

    print(f"Wrote {len(defaults)} defaults to {out_path}", file=sys.stderr)
    print(json.dumps(defaults, indent=2))


if __name__ == "__main__":
    main()
