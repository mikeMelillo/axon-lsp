import os
import sys
import json
from dotenv import load_dotenv

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from server.axon_lsp.server import TrioParser, FantomParser

load_dotenv()

GITHUB_BASE = os.getenv("GITHUB_BASE", "https://github.com/haxall/haxall/blob/3.1.12")
LOCAL_PREFIX = os.getenv("LOCAL_PREFIX")

CORE_FUNCS = "cache_sources/coreFuncs.trio"
HAXALL_PATH = "cache_sources/haxall"
OUTPUT_PATH = os.path.join(
    os.path.dirname(__file__), "..", "server", "axon_lsp", "function_cache.json"
)


def serialize_func(func):
    """Convert function dict to JSON-serializable format."""
    result = {
        "name": func.get("name"),
        "doc": func.get("doc", ""),
        "args_str": func.get("args_str", "()"),
        "params": func.get("params", []),
        "kind": func.get("kind"),
    }
    # Handle location if present
    if func.get("location"):
        loc = func["location"]
        uri = str(loc.uri) if hasattr(loc, 'uri') else str(loc)
        
        if uri.endswith("coreFuncs.trio"):
            uri = "axon-ext://coreFuncs.trio"
        elif uri.startswith("file://" + LOCAL_PREFIX):
            rel_path = uri[len("file://" + LOCAL_PREFIX):]
            uri = f"{GITHUB_BASE}/{rel_path}"

        result["location"] = {"uri": uri}
    return result


def build_cache():
    functions = {}

    # Index .trio files
    trio_count = 0
    fan_count = 0

    functions.update(TrioParser.parse_file(CORE_FUNCS))
    trio_count += 1

    for root, _, files in os.walk(HAXALL_PATH):
        for file in files:
            path = os.path.join(root, file)
            if file.endswith(".trio"):
                functions.update(TrioParser.parse_file(path))
                trio_count += 1
            elif file.endswith(".fan"):
                functions.update(FantomParser.parse_file(path))
                fan_count += 1

    # Convert to list for JSON serialization
    func_list = [serialize_func(f) for f in functions.values()]

    # Write to output
    with open(OUTPUT_PATH, "w") as f:
        json.dump(func_list, f, indent=2)

    print(f"Wrote {len(func_list)} functions to {OUTPUT_PATH}")
    print(f"  .trio files: {trio_count}")
    print(f"  .fan files: {fan_count}")


if __name__ == "__main__":
    build_cache()
