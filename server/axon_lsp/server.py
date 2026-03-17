import sys
import os
import re
import json
import logging

# import pickle
# import subprocess
from typing import Dict, List, Optional, Union
from pathlib import Path

try:
    from pygls.server import LanguageServer
except ImportError:
    from pygls import LanguageServer
from lsprotocol.types import (
    TEXT_DOCUMENT_COMPLETION,
    TEXT_DOCUMENT_DID_SAVE,
    TEXT_DOCUMENT_DID_OPEN,
    TEXT_DOCUMENT_DID_CHANGE,
    TEXT_DOCUMENT_DEFINITION,
    TEXT_DOCUMENT_HOVER,
    INITIALIZED,
    CompletionItem,
    CompletionItemKind,
    CompletionList,
    CompletionParams,
    InitializeParams,
    DidSaveTextDocumentParams,
    DidOpenTextDocumentParams,
    DidChangeTextDocumentParams,
    Location,
    Position,
    Range,
    DefinitionParams,
    Diagnostic,
    DiagnosticSeverity,
    Hover,
    MarkupContent,
    MarkupKind,
    HoverParams,
    Command,
)

# Set up logging to stderr so pygls captures it for VS Code Output
logging.basicConfig(
    level=logging.DEBUG,
    format="%(asctime)s - %(levelname)s - %(message)s",
    handlers=[logging.StreamHandler(sys.stderr)],
)
logger = logging.getLogger("axon-lsp")

VERSION = "0.1.4"


class FantomParser:
    """Parses .fan files to find @Axon decorated functions."""

    @staticmethod
    def parse_file(filepath: str) -> Dict[str, dict]:
        found_funcs = {}
        try:
            with open(filepath, "r", encoding="utf-8") as f:
                lines = f.readlines()

            uri = f"file://{os.path.abspath(filepath)}"

            for i, line in enumerate(lines):
                if "@Axon" in line:
                    search_area = "".join(lines[i : i + 3])
                    func_match = re.search(
                        r"(?:static\s+)?(?:\w+\s+)?(\w+)\s*\((.*?)\)", search_area
                    )

                    if func_match:
                        name = func_match.group(1)
                        raw_args = func_match.group(2).strip()

                        clean_args = []
                        if raw_args:
                            for arg in raw_args.split(","):
                                parts = arg.strip().split(" ")
                                clean_args.append(parts[-1])

                        args_str = f"({', '.join(clean_args)})"

                        doc_lines = []
                        for j in range(i - 1, -1, -1):
                            prev_line = lines[j].strip()
                            if prev_line.startswith("**"):
                                doc_lines.insert(0, prev_line[2:].strip())
                            elif not prev_line:
                                continue
                            else:
                                break

                        doc = (
                            "\n".join(doc_lines)
                            if doc_lines
                            else "No documentation available."
                        )

                        found_funcs[name] = {
                            "name": name,
                            "doc": doc,
                            "args_str": args_str,
                            "params": clean_args,
                            "kind": CompletionItemKind.Function,
                            "location": Location(
                                uri=uri,
                                range=Range(
                                    start=Position(
                                        line=i, character=line.find("@Axon")
                                    ),
                                    end=Position(
                                        line=i,
                                        character=line.find("@Axon") + 5 + len(name),
                                    ),
                                ),
                            ),
                        }
            return found_funcs
        except Exception as e:
            logger.error(f"Error parsing Fantom file {filepath}: {e}")
            return {}


class TrioParser:
    @staticmethod
    def parse_file(filepath: str):
        try:
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()

            uri = f"file://{os.path.abspath(filepath)}"
            records = re.split(r"\n-{3,}\s*(?=\n|$)", content)
            found_funcs = {}
            current_line = 0

            for record in records:
                lines = record.split("\n")
                tags = {}
                name_line_offset = 0
                name_char_offset = 0
                last_tag = None

                for i, line in enumerate(lines):
                    tag_match = re.match(r"^([a-zA-Z0-9_]+):(.*)", line)
                    if tag_match:
                        last_tag = tag_match.group(1).strip()
                        val = tag_match.group(2).strip().strip('"')
                        tags[last_tag] = val
                        if last_tag == "name":
                            name_line_offset = i
                            name_char_offset = line.find(val)
                        if last_tag == "func":
                            name_line_offset = i
                            name_char_offset = line.find("func")
                    elif line.startswith("  ") and last_tag:
                        tags[last_tag] = tags[last_tag] + "\n" + line[2:]
                    elif line.strip() and not line.startswith(" "):
                        marker = line.strip()
                        tags[marker] = True
                        last_tag = marker

                if "func" in tags and "name" in tags:
                    name = tags["name"]
                    doc = tags.get(
                        "doc", tags.get("summary", "No documentation available.")
                    )
                    src = tags.get("src", "")

                    defcomp_match = re.search(r"defcomp\s+", src)
                    if defcomp_match:
                        params_list, args_str, return_type = TrioParser._parse_defcomp(
                            src
                        )
                    else:
                        arg_match = re.search(r"^\s*\((.*?)\)\s*=>", src, re.MULTILINE)
                        if arg_match:
                            args_content = arg_match.group(1).strip()
                            args_str = f"({args_content})"
                            if args_content:
                                params_list = [
                                    p.strip() for p in args_content.split(",")
                                ]
                            else:
                                params_list = []
                        else:
                            args_str = "()"
                            params_list = []
                        return_type = None

                    abs_path = Path(filepath).resolve()
                    uri = abs_path.as_uri()

                    found_funcs[name] = {
                        "name": name,
                        "doc": doc,
                        "args_str": args_str,
                        "params": params_list,
                        "kind": CompletionItemKind.Function,
                        "location": Location(
                            uri=uri,
                            range=Range(
                                start=Position(
                                    line=current_line + name_line_offset,
                                    character=name_char_offset,
                                ),
                                end=Position(
                                    line=current_line + name_line_offset,
                                    character=name_char_offset + len(name),
                                ),
                            ),
                        ),
                    }
                    if return_type:
                        found_funcs[name]["return_type"] = return_type
                current_line += len(lines)
            return found_funcs
        except Exception as e:
            logger.error(f"Error parsing Trio {filepath}: {e}")
            return {}

    @staticmethod
    def _parse_defcomp(src: str):
        defcomp_match = re.search(r"defcomp\s+(.*?)\s*do\s*\n", src, re.DOTALL)
        if not defcomp_match:
            return [], "()", None

        defcomp_block = defcomp_match.group(1)
        input_params = []
        output_fields = []

        for line in defcomp_block.split("\n"):
            line = line.strip()
            if not line or ":" not in line:
                continue

            colon_idx = line.index(":")
            field_name = line[:colon_idx].strip()
            field_value = line[colon_idx + 1 :].strip()

            if "readonly" in field_value.lower():
                output_fields.append(field_name)
            else:
                input_params.append(f"{field_name}: Obj?")

        if output_fields:
            output_str = ", ".join([f"{f}: Obj?" for f in output_fields])
            return_type = f"dict({{{output_str}}})"
        else:
            return_type = "dict({})"

        params_str = ", ".join(input_params)
        args_str = f"( {{{params_str}}} )"

        return input_params, args_str, return_type


class NamespaceManager:
    def __init__(self):
        self.core_funcs: Dict[str, dict] = {}
        self.external_funcs: Dict[str, dict] = {}
        self.local_funcs: Dict[str, dict] = {}
        self.references_map: Dict[str, List[Location]] = {}

        # Load static core functions from bundled backstop
        self._load_core()

    def _load_core(self):
        # Load from static backstop file (shipped with extension)
        self._apply_default_backstop()
        logger.info(
            f"Loaded {len(self.core_funcs)} core functions from static backstop"
        )

    def _apply_default_backstop(self):
        """Load functions from bundled JSON cache."""
        cache_path = os.path.join(os.path.dirname(__file__), "function_cache.json")

        if os.path.exists(cache_path):
            try:
                with open(cache_path, "r") as f:
                    func_list = json.load(f)

                for func in func_list:
                    name = func.get("name")
                    kind_val = func.get("kind")
                    if isinstance(kind_val, int):
                        kind_val = CompletionItemKind(kind_val)
                    if name:
                        loc_uri = func.get("location", {}).get("uri")
                        if loc_uri == "axon-ext://coreFuncs.trio":
                            ext_path = Path(__file__).parent / "coreFuncs.trio"
                            loc_uri = ext_path.as_uri()
                        # Reconstruct function dict with proper types
                        if loc_uri:
                            location = Location(
                                uri=loc_uri,
                                range=Range(
                                    start=Position(line=0, character=0),
                                    end=Position(line=0, character=0),
                                ),
                            )
                        else:
                            location = None
                        self.core_funcs[name] = {
                            "name": name,
                            "doc": func.get("doc", ""),
                            "args_str": func.get("args_str", "()"),
                            "params": func.get("params", []),
                            "kind": kind_val,
                            "location": location,
                        }
                logger.info(f"Loaded {len(self.core_funcs)} functions from cache")
            except Exception as e:
                logger.warning(f"Failed to load cache: {e}")
        else:
            logger.warning(f"Cache file not found: {cache_path}")

    def add_reference(self, name: str, location: Location):
        if name not in self.references_map:
            self.references_map[name] = []
        self.references_map[name].append(location)

    def clear_references_for_uri(self, uri: str):
        for name in list(self.references_map.keys()):
            self.references_map[name] = [
                loc for loc in self.references_map[name] if loc.uri != uri
            ]

    def update_local_index(self, workspace_root: str):
        logger.info(f"Scanning workspace: {workspace_root}")
        new_local = {}
        for root, _, files in os.walk(workspace_root):
            for file in files:
                if file.endswith((".trio", ".axon")):
                    new_local.update(TrioParser.parse_file(os.path.join(root, file)))
                elif file.endswith((".fan")):
                    new_local.update(FantomParser.parse_file(os.path.join(root, file)))
        self.local_funcs = new_local

    def get_completions(self) -> List[CompletionItem]:
        # Priority: Workspace > External Libs > Core
        merged = {**self.core_funcs, **self.external_funcs, **self.local_funcs}
        return [
            CompletionItem(
                label=f["name"],
                kind=f["kind"],
                detail=f["name"]
                + f["args_str"]
                + (f" // -> {f['return_type']}" if f.get("return_type") else ""),
                documentation=f["doc"],
            )
            for f in merged.values()
        ]

    def get_definition(self, symbol: str) -> Optional[Union[Location, Command]]:
        loc = (
            self.local_funcs.get(symbol, {}).get("location")
            or self.external_funcs.get(symbol, {}).get("location")
            or self.core_funcs.get(symbol, {}).get("location")
        )

        if loc and loc.uri and loc.uri.startswith("https://"):
            return Command(
                title="Open in GitHub",
                command="extension.openExternal",
                arguments=[loc.uri],
            )

        return loc

    def find_function(self, name: str) -> Optional[dict]:
        return (
            self.local_funcs.get(name)
            or self.external_funcs.get(name)
            or self.core_funcs.get(name)
        )


# Create server instance
server = LanguageServer("axon-lsp-server", VERSION)
manager: Optional[NamespaceManager] = None


class Validator:
    @staticmethod
    def _parse_local_functions(source: str) -> set:
        """Parse function definitions from source to identify local/helper functions."""
        local_funcs = set()
        lines = source.split("\n")

        for i, line in enumerate(lines):
            stripped = line.strip()

            # Match function definitions: name: (args) =>
            # e.g., foo: (a,b) => return a + b
            func_match = re.match(r"^(\w+)\s*:\s*\([^)]*\)\s*=>", stripped)
            if func_match:
                func_name = func_match.group(1)
                # Skip if line contains quotes (string literals)
                if '"' not in stripped and "'" not in stripped:
                    local_funcs.add(func_name)

            # Match lambda expressions assigned to names: name: (args) =>
            lambda_match = re.match(r"^(\w+)\s*:\s*\(", stripped)
            if lambda_match:
                func_name = lambda_match.group(1)
                # Skip if line contains quotes (string literals)
                if '"' not in stripped and "'" not in stripped:
                    local_funcs.add(func_name)

        return local_funcs

    @staticmethod
    def validate(ls: LanguageServer, uri: str, mgr: NamespaceManager):
        doc = ls.workspace.get_text_document(uri)
        mgr.clear_references_for_uri(uri)
        diagnostics = []

        # Parse local functions from this file
        local_funcs = Validator._parse_local_functions(doc.source)

        for i, line in enumerate(doc.source.splitlines()):
            for match in re.finditer(r"\b([a-zA-Z0-9_]+)\b", line):
                name = match.group(1)

                # Skip if inside a string literal (quoted string)
                quote_count_before = line[: match.start()].count('"')
                if quote_count_before % 2 == 1:
                    continue

                if name in ["if", "do", "return", "try", "catch", "throw"]:
                    continue

                # Check if it's a local function defined in this file
                is_local = name in local_funcs

                f = mgr.find_function(name)
                if f:
                    mgr.add_reference(
                        name,
                        Location(
                            uri=uri,
                            range=Range(
                                start=Position(line=i, character=match.start()),
                                end=Position(line=i, character=match.end()),
                            ),
                        ),
                    )

                # Only report as undefined if:
                # 1. It looks like a function call (followed by parenthesis)
                # 2. It's NOT defined globally
                # 3. It's NOT a local function defined in this file
                if (
                    match.end() < len(line)
                    and line[match.end()] == "("
                    and not f
                    and not is_local
                ):
                    diagnostics.append(
                        Diagnostic(
                            range=Range(
                                start=Position(line=i, character=match.start()),
                                end=Position(line=i, character=match.end()),
                            ),
                            message=f"Undefined function: {name}",
                            severity=DiagnosticSeverity.Error,
                        )
                    )
        ls.publish_diagnostics(uri, diagnostics)


@server.feature(INITIALIZED)
def on_initialized(ls: LanguageServer, params: InitializeParams):
    global manager
    logger.info("LSP Initialized - Loading Namespaces...")

    manager = NamespaceManager()
    if ls.workspace.root_path:
        manager.update_local_index(ls.workspace.root_path)


@server.feature(TEXT_DOCUMENT_DID_OPEN)
def did_open(ls: LanguageServer, params: DidOpenTextDocumentParams):
    if manager:
        Validator.validate(ls, params.text_document.uri, manager)


@server.feature(TEXT_DOCUMENT_DID_CHANGE)
def did_change(ls: LanguageServer, params: DidChangeTextDocumentParams):
    if manager:
        Validator.validate(ls, params.text_document.uri, manager)


@server.feature(TEXT_DOCUMENT_DID_SAVE)
def did_save(ls: LanguageServer, params: DidSaveTextDocumentParams):
    if manager:
        if ls.workspace.root_path:
            manager.update_local_index(ls.workspace.root_path)
        Validator.validate(ls, params.text_document.uri, manager)


@server.feature(TEXT_DOCUMENT_DEFINITION)
def definition(ls: LanguageServer, params: DefinitionParams):
    if not manager:
        return None
    doc = ls.workspace.get_text_document(params.text_document.uri)
    line = doc.source.splitlines()[params.position.line]
    words = re.finditer(r"\b[a-zA-Z0-9_]+\b", line)
    for m in words:
        if m.start() <= params.position.character <= m.end():
            return manager.get_definition(m.group())
    return None


@server.feature(TEXT_DOCUMENT_HOVER)
def hover(ls: LanguageServer, params: HoverParams) -> Optional[Hover]:
    if not manager:
        return None
    doc = ls.workspace.get_text_document(params.text_document.uri)
    line = doc.source.splitlines()[params.position.line]
    words = re.finditer(r"\b[a-zA-Z0-9_]+\b", line)
    for m in words:
        if m.start() <= params.position.character <= m.end():
            f = manager.find_function(m.group())
            if f:
                return Hover(
                    contents=MarkupContent(
                        kind=MarkupKind.Markdown,
                        value=f"**{f['name']}{f['args_str']}**\n\n---\n\n{f['doc']}",
                    )
                )
    return None


@server.feature(TEXT_DOCUMENT_COMPLETION)
def completions(params: CompletionParams) -> CompletionList:
    if not manager:
        return CompletionList(is_incomplete=False, items=[])
    return CompletionList(is_incomplete=False, items=manager.get_completions())


if __name__ == "__main__":
    server.start_io()
