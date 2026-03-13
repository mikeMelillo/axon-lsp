import sys
import os
import re
import logging
import pickle
import subprocess
from typing import Dict, List, Optional
from pygls.server import LanguageServer
from lsprotocol.types import (
    TEXT_DOCUMENT_COMPLETION,
    TEXT_DOCUMENT_DID_SAVE,
    TEXT_DOCUMENT_DID_OPEN,
    TEXT_DOCUMENT_DID_CHANGE,
    TEXT_DOCUMENT_DEFINITION,
    # TEXT_DOCUMENT_SIGNATURE_HELP,
    TEXT_DOCUMENT_HOVER,
    # TEXT_DOCUMENT_REFERENCES,
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
    # ReferenceParams,
    # SignatureHelp,
    # SignatureInformation,
    # ParameterInformation,
    # SignatureHelpParams,
    # SignatureHelpOptions,
    Diagnostic,
    DiagnosticSeverity,
    Hover,
    MarkupContent,
    MarkupKind,
    HoverParams,
)

# Set up logging to stderr so pygls captures it for VS Code Output
logging.basicConfig(
    level=logging.DEBUG,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[logging.StreamHandler(sys.stderr)]
)
logger = logging.getLogger("axon-lsp")

VERSION = "0.1.3"
HAXALL_REPO_PATH = os.environ.get("HAXALL_HOME", "/home/mike/work/haxall-3.1.12/haxall")
# Support for external libraries via environment variable (colon/semicolon separated)
EXTERNAL_PATHS = os.environ.get("AXON_PATH", "/home/mike/work/skyspark-sba-core").split(os.pathsep)

class FantomParser:
    """Parses .fan files to find @Axon decorated functions."""
    @staticmethod
    def parse_file(filepath: str) -> Dict[str, dict]:
        found_funcs = {}
        try:
            with open(filepath, 'r', encoding='utf-8') as f:
                lines = f.readlines()

            uri = f"file://{os.path.abspath(filepath)}"
            
            for i, line in enumerate(lines):
                if '@Axon' in line:
                    search_area = "".join(lines[i:i+3])
                    func_match = re.search(r'(?:static\s+)?(?:\w+\s+)?(\w+)\s*\((.*?)\)', search_area)
                    
                    if func_match:
                        name = func_match.group(1)
                        raw_args = func_match.group(2).strip()
                        
                        clean_args = []
                        if raw_args:
                            for arg in raw_args.split(','):
                                parts = arg.strip().split(' ')
                                clean_args.append(parts[-1])
                        
                        args_str = f"({', '.join(clean_args)})"
                        
                        doc_lines = []
                        for j in range(i-1, -1, -1):
                            prev_line = lines[j].strip()
                            if prev_line.startswith('**'):
                                doc_lines.insert(0, prev_line[2:].strip())
                            elif not prev_line:
                                continue
                            else:
                                break
                        
                        doc = "\n".join(doc_lines) if doc_lines else "No documentation available."
                        
                        found_funcs[name] = {
                            'name': name,
                            'doc': doc,
                            'args_str': args_str,
                            'params': clean_args,
                            'kind': CompletionItemKind.Function,
                            'location': Location(
                                uri=uri,
                                range=Range(
                                    start=Position(line=i, character=line.find('@Axon')),
                                    end=Position(line=i, character=line.find('@Axon') + 5)
                                )
                            )
                        }
            return found_funcs
        except Exception as e:
            logger.error(f"Error parsing Fantom file {filepath}: {e}")
            return {}

class TrioParser:
    @staticmethod
    def parse_file(filepath: str):
        try:
            with open(filepath, 'r', encoding='utf-8') as f:
                content = f.read()
            
            uri = f"file://{os.path.abspath(filepath)}"
            records = re.split(r'\n-{3,}\s*(?=\n|$)', content)
            found_funcs = {}
            current_line = 0
            
            for record in records:
                lines = record.split('\n')
                tags = {}
                name_line_offset = 0
                name_char_offset = 0
                last_tag = None
                
                for i, line in enumerate(lines):
                    tag_match = re.match(r'^([a-zA-Z0-9_]+):(.*)', line)
                    if tag_match:
                        last_tag = tag_match.group(1).strip()
                        val = tag_match.group(2).strip().strip('"')
                        tags[last_tag] = val
                        if last_tag == 'name':
                            name_line_offset = i
                            name_char_offset = line.find(val)
                    elif line.startswith('  ') and last_tag:
                        tags[last_tag] = tags[last_tag] + '\n' + line[2:]
                    elif line.strip() and not line.startswith(' '):
                        marker = line.strip()
                        tags[marker] = True
                        last_tag = marker

                if 'func' in tags and 'name' in tags:
                    name = tags['name']
                    doc = tags.get('doc', tags.get('summary', 'No documentation available.'))
                    src = tags.get('src', '')
                    args_str = ""
                    params_list = []
                    arg_match = re.search(r'^\s*\((.*?)\)\s*=>', src, re.MULTILINE)
                    if arg_match:
                        args_content = arg_match.group(1).strip()
                        args_str = f"({args_content})"
                        if args_content:
                            params_list = [p.strip() for p in args_content.split(',')]
                    
                    found_funcs[name] = {
                        'name': name,
                        'doc': doc,
                        'args_str': args_str,
                        'params': params_list,
                        'kind': CompletionItemKind.Function,
                        'location': Location(
                            uri=uri,
                            range=Range(
                                start=Position(line=current_line + name_line_offset, character=name_char_offset),
                                end=Position(line=current_line + name_line_offset, character=name_char_offset + len(name))
                            )
                        )
                    }
                current_line += len(lines)
            return found_funcs
        except Exception as e:
            logger.error(f"Error parsing Trio {filepath}: {e}")
            return {}

class NamespaceManager:
    def __init__(self):
        self.core_funcs: Dict[str, dict] = {}
        self.external_funcs: Dict[str, dict] = {}
        self.local_funcs: Dict[str, dict] = {}
        self.references_map: Dict[str, List[Location]] = {}
        self._cache_dir = os.path.join(os.path.expanduser("~"), ".axon_lsp_cache")
        self._cache_file = os.path.join(self._cache_dir, f"core_index_v{VERSION}.pkl")
        
        self.current_cache_id = self._generate_cache_id()
        self._load_core()
        self._load_external_libraries()

    def _generate_cache_id(self) -> str:
        if not HAXALL_REPO_PATH or not os.path.exists(HAXALL_REPO_PATH):
            return "minimal-fallback"
        try:
            git_dir = os.path.join(HAXALL_REPO_PATH, ".git")
            if os.path.exists(git_dir):
                commit = subprocess.check_output(
                    ["git", "rev-parse", "HEAD"], 
                    cwd=HAXALL_REPO_PATH, 
                    stderr=subprocess.DEVNULL
                ).decode().strip()
                return f"git-{commit}"
        except Exception:
            pass
        try:
            mtime = os.path.getmtime(HAXALL_REPO_PATH)
            return f"path-{HAXALL_REPO_PATH}-{mtime}"
        except Exception:
            return "unknown-env"

    def _load_core(self):
        if os.path.exists(self._cache_file):
            try:
                with open(self._cache_file, 'rb') as f:
                    cache_data = pickle.load(f)
                cached_id = cache_data.get('cache_id')
                if cached_id == self.current_cache_id and cached_id != "minimal-fallback":
                    self.core_funcs = cache_data.get('functions', {})
                    logger.info(f"Core Cache hit [{cached_id}]: {len(self.core_funcs)} functions.")
                    return
            except Exception as e:
                logger.error(f"Failed to load cache: {e}")

        if HAXALL_REPO_PATH and os.path.exists(HAXALL_REPO_PATH) and self.current_cache_id != "minimal-fallback":
            logger.info(f"Indexing Haxall Repo at {HAXALL_REPO_PATH}")
            for root, _, files in os.walk(HAXALL_REPO_PATH):
                for file in files:
                    path = os.path.join(root, file)
                    if file.endswith('.trio'):
                        self.core_funcs.update(TrioParser.parse_file(path))
                    elif file.endswith('.fan'):
                        self.core_funcs.update(FantomParser.parse_file(path))
            self._save_cache()
        else:
            for s in ["read", "readAll", "hisRead", "hisWrite", "filter", "map", "now"]:
                self.core_funcs[s] = {'name': s, 'doc': 'Fallback', 'args_str': '()', 'params': [], 'kind': CompletionItemKind.Method, 'location': None}

    def _load_external_libraries(self):
        """Indexes libraries provided in AXON_PATH."""
        self.external_funcs = {}
        valid_paths = [p for p in EXTERNAL_PATHS if p.strip() and os.path.exists(p)]
        if not valid_paths:
            return

        logger.info(f"Indexing External Libraries from AXON_PATH: {valid_paths}")
        for path in valid_paths:
            for root, _, files in os.walk(path):
                for file in files:
                    full_path = os.path.join(root, file)
                    if file.endswith(('.trio', '.axon')):
                        self.external_funcs.update(TrioParser.parse_file(full_path))
                    elif file.endswith('.fan'):
                        self.external_funcs.update(FantomParser.parse_file(full_path))
        logger.info(f"Indexed {len(self.external_funcs)} external library functions.")

    def _save_cache(self):
        try:
            os.makedirs(self._cache_dir, exist_ok=True)
            cache_data = {'cache_id': self.current_cache_id, 'functions': self.core_funcs}
            with open(self._cache_file, 'wb') as f:
                pickle.dump(cache_data, f)
        except Exception as e:
            logger.error(f"Save cache failed: {e}")

    def clear_references_for_uri(self, uri: str):
        for name in list(self.references_map.keys()):
            self.references_map[name] = [loc for loc in self.references_map[name] if loc.uri != uri]

    def add_reference(self, name: str, location: Location):
        if name not in self.references_map:
            self.references_map[name] = []
        self.references_map[name].append(location)

    def update_local_index(self, workspace_root: str):
        logger.info(f"Scanning workspace: {workspace_root}")
        new_local = {}
        for root, _, files in os.walk(workspace_root):
            for file in files:
                if file.endswith(('.trio', '.axon')):
                    new_local.update(TrioParser.parse_file(os.path.join(root, file)))
        self.local_funcs = new_local

    def get_completions(self) -> List[CompletionItem]:
        # Priority: Workspace > External Libs > Core
        merged = {**self.core_funcs, **self.external_funcs, **self.local_funcs}
        return [CompletionItem(label=f['name'], kind=f['kind'], detail=f"{f['name']}{f['args_str']}", documentation=f['doc']) for f in merged.values()]

    def get_definition(self, symbol: str) -> Optional[Location]:
        return self.local_funcs.get(symbol, {}).get('location') or \
               self.external_funcs.get(symbol, {}).get('location') or \
               self.core_funcs.get(symbol, {}).get('location')

    def find_function(self, name: str) -> Optional[dict]:
        return self.local_funcs.get(name) or self.external_funcs.get(name) or self.core_funcs.get(name)

# Create server instance
server = LanguageServer("axon-lsp-server", VERSION)
manager: Optional[NamespaceManager] = None

class Validator:
    @staticmethod
    def validate(ls: LanguageServer, uri: str, mgr: NamespaceManager):
        doc = ls.workspace.get_text_document(uri)
        mgr.clear_references_for_uri(uri)
        diagnostics = []
        for i, line in enumerate(doc.source.splitlines()):
            for match in re.finditer(r'\b([a-zA-Z0-9_]+)\b', line):
                name = match.group(1)
                if name in ['if', 'do', 'return', 'try', 'catch', 'throw']: continue
                f = mgr.find_function(name)
                if f: 
                    mgr.add_reference(name, Location(uri=uri, range=Range(start=Position(line=i, character=match.start()), end=Position(line=i, character=match.end()))))
                
                if match.end() < len(line) and line[match.end()] == '(' and not f:
                    diagnostics.append(Diagnostic(
                        range=Range(start=Position(line=i, character=match.start()), end=Position(line=i, character=match.end())), 
                        message=f"Undefined function: {name}", 
                        severity=DiagnosticSeverity.Error
                    ))
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
    if manager: Validator.validate(ls, params.text_document.uri, manager)

@server.feature(TEXT_DOCUMENT_DID_CHANGE)
def did_change(ls: LanguageServer, params: DidChangeTextDocumentParams):
    if manager: Validator.validate(ls, params.text_document.uri, manager)

@server.feature(TEXT_DOCUMENT_DID_SAVE)
def did_save(ls: LanguageServer, params: DidSaveTextDocumentParams):
    if manager:
        if ls.workspace.root_path: manager.update_local_index(ls.workspace.root_path)
        Validator.validate(ls, params.text_document.uri, manager)

@server.feature(TEXT_DOCUMENT_DEFINITION)
def definition(ls: LanguageServer, params: DefinitionParams):
    if not manager: return None
    doc = ls.workspace.get_text_document(params.text_document.uri)
    line = doc.source.splitlines()[params.position.line]
    words = re.finditer(r'\b[a-zA-Z0-9_]+\b', line)
    for m in words:
        if m.start() <= params.position.character <= m.end():
            return manager.get_definition(m.group())
    return None

@server.feature(TEXT_DOCUMENT_HOVER)
def hover(ls: LanguageServer, params: HoverParams) -> Optional[Hover]:
    if not manager: return None
    doc = ls.workspace.get_text_document(params.text_document.uri)
    line = doc.source.splitlines()[params.position.line]
    words = re.finditer(r'\b[a-zA-Z0-9_]+\b', line)
    for m in words:
        if m.start() <= params.position.character <= m.end():
            f = manager.find_function(m.group())
            if f:
                return Hover(contents=MarkupContent(kind=MarkupKind.Markdown, value=f"**{f['name']}{f['args_str']}**\n\n---\n\n{f['doc']}"))
    return None

@server.feature(TEXT_DOCUMENT_COMPLETION)
def completions(params: CompletionParams) -> CompletionList:
    if not manager: return CompletionList(is_incomplete=False, items=[])
    return CompletionList(is_incomplete=False, items=manager.get_completions())

if __name__ == "__main__":
    server.start_io()