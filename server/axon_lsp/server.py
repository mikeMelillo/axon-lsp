import sys
import os
import re
import logging
import pickle
from typing import Dict, List, Optional
from pygls.server import LanguageServer
from lsprotocol.types import (
    TEXT_DOCUMENT_COMPLETION,
    TEXT_DOCUMENT_DID_SAVE,
    TEXT_DOCUMENT_DID_OPEN,
    TEXT_DOCUMENT_DID_CHANGE,
    TEXT_DOCUMENT_DEFINITION,
    TEXT_DOCUMENT_SIGNATURE_HELP,
    TEXT_DOCUMENT_HOVER,
    TEXT_DOCUMENT_REFERENCES,
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
    ReferenceParams,
    SignatureHelp,
    SignatureInformation,
    ParameterInformation,
    SignatureHelpParams,
    SignatureHelpOptions,
    Diagnostic,
    DiagnosticSeverity,
    Hover,
    MarkupContent,
    MarkupKind,
    HoverParams,
)

# Set up logging
logging.basicConfig(filename="axon_lsp.log", level=logging.DEBUG, filemode="w")
logger = logging.getLogger("axon-lsp")

VERSION = "0.1.0"
# Set this to your local Haxall path, or use environment variable
HAXALL_REPO_PATH = os.environ.get("HAXALL_HOME", "/home/mike/work/haxall-3.1.12/haxall")

class TrioParser:
    """Parses .trio files to extract function definitions and metadata."""
    
    @staticmethod
    def parse_file(filepath: str):
        """Parses a file and returns funcs with location data."""
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
            logger.error(f"Error parsing {filepath}: {e}")
            return {}

class NamespaceManager:
    def __init__(self):
        self.core_funcs: Dict[str, dict] = {}
        self.local_funcs: Dict[str, dict] = {}
        self.references_map: Dict[str, List[Location]] = {}
        self._cache_dir = os.path.join(os.path.expanduser("~"), ".axon_lsp_cache")
        self._cache_file = os.path.join(self._cache_dir, f"core_index_{VERSION}.pkl")
        self._load_core()

    def _load_core(self):
        """Loads core library functions from cache or by scanning HAXALL_REPO_PATH."""
        if os.path.exists(self._cache_file):
            try:
                with open(self._cache_file, 'rb') as f:
                    self.core_funcs = pickle.load(f)
                logger.info(f"Loaded {len(self.core_funcs)} core functions from cache.")
                return
            except Exception as e:
                logger.error(f"Failed to load cache: {e}")

        if HAXALL_REPO_PATH and os.path.exists(HAXALL_REPO_PATH):
            logger.info(f"Scanning Haxall Repo at {HAXALL_REPO_PATH}...")
            # Scan for all .trio files in the repo (usually in src/ folders)
            for root, _, files in os.walk(HAXALL_REPO_PATH):
                for file in files:
                    if file.endswith('.trio'):
                        path = os.path.join(root, file)
                        funcs = TrioParser.parse_file(path)
                        self.core_funcs.update(funcs)
            
            if self.core_funcs:
                self._save_cache()
                logger.info(f"Indexed {len(self.core_funcs)} functions from Haxall repo.")
        else:
            logger.warning("HAXALL_REPO_PATH not set or invalid. Using minimal defaults.")
            standards = ["read", "readAll", "hisRead", "hisWrite", "filter", "map", "fold", "now", "today"]
            for s in standards:
                self.core_funcs[s] = {
                    'name': s,
                    'doc': f"Standard Haxall function: {s}",
                    'args_str': "(...) ",
                    'params': ["..."],
                    'kind': CompletionItemKind.Method,
                    'location': None
                }

    def _save_cache(self):
        try:
            if not os.path.exists(self._cache_dir):
                os.makedirs(self._cache_dir)
            with open(self._cache_file, 'wb') as f:
                pickle.dump(self.core_funcs, f)
            logger.info("Core index cached successfully.")
        except Exception as e:
            logger.error(f"Failed to save cache: {e}")

    def clear_references_for_uri(self, uri: str):
        for name in self.references_map:
            self.references_map[name] = [loc for loc in self.references_map[name] if loc.uri != uri]

    def add_reference(self, name: str, location: Location):
        if name not in self.references_map:
            self.references_map[name] = []
        self.references_map[name].append(location)

    def update_local_index(self, workspace_root: str):
        logger.info(f"Re-scanning workspace: {workspace_root}")
        new_local_index = {}
        for root, _, files in os.walk(workspace_root):
            for file in files:
                if file.endswith('.trio'):
                    path = os.path.join(root, file)
                    funcs = TrioParser.parse_file(path)
                    new_local_index.update(funcs)
        self.local_funcs = new_local_index

    def get_completions(self) -> List[CompletionItem]:
        merged = {**self.core_funcs, **self.local_funcs}
        return [
            CompletionItem(
                label=f['name'],
                kind=f['kind'],
                detail=f"{f['name']}{f['args_str']}",
                documentation=f['doc']
            ) for f in merged.values()
        ]
    
    def get_definition(self, symbol: str) -> Optional[Location]:
        if symbol in self.local_funcs:
            return self.local_funcs[symbol].get('location')
        if symbol in self.core_funcs:
            return self.core_funcs[symbol].get('location')
        return None

    def get_references(self, symbol: str) -> List[Location]:
        return self.references_map.get(symbol, [])

    def find_function(self, name: str) -> Optional[dict]:
        merged = {**self.core_funcs, **self.local_funcs}
        return merged.get(name)

class Validator:
    @staticmethod
    def validate(ls: LanguageServer, uri: str, manager: NamespaceManager):
        doc = ls.workspace.get_document(uri)
        diagnostics = []
        manager.clear_references_for_uri(uri)
        
        lines = doc.source.splitlines()
        for i, line in enumerate(lines):
            calls = re.finditer(r'\b([a-zA-Z0-9_]+)\b', line)
            for match in calls:
                name = match.group(1)
                if name in ['if', 'do', 'return', 'try', 'catch']:
                    continue
                
                is_known = manager.find_function(name)
                is_call = match.end() < len(line) and line[match.end()] == '('
                
                if is_known:
                    loc = Location(
                        uri=uri,
                        range=Range(
                            start=Position(line=i, character=match.start(1)),
                            end=Position(line=i, character=match.end(1))
                        )
                    )
                    manager.add_reference(name, loc)
                
                if is_call and not is_known:
                    d = Diagnostic(
                        range=Range(
                            start=Position(line=i, character=match.start(1)),
                            end=Position(line=i, character=match.end(1))
                        ),
                        message=f"Undefined function: '{name}'",
                        severity=DiagnosticSeverity.Error,
                        source="Axon LSP"
                    )
                    diagnostics.append(d)
        
        ls.publish_diagnostics(uri, diagnostics)

server = LanguageServer("axon-lsp-server", VERSION)
manager = NamespaceManager()

@server.feature(INITIALIZED)
def on_initialized(ls: LanguageServer, params: InitializeParams):
    if ls.workspace.root_path:
        manager.update_local_index(ls.workspace.root_path)

@server.feature(TEXT_DOCUMENT_DID_OPEN)
def did_open(ls: LanguageServer, params: DidOpenTextDocumentParams):
    Validator.validate(ls, params.text_document.uri, manager)

@server.feature(TEXT_DOCUMENT_DID_CHANGE)
def did_change(ls: LanguageServer, params: DidChangeTextDocumentParams):
    Validator.validate(ls, params.text_document.uri, manager)

@server.feature(TEXT_DOCUMENT_DID_SAVE)
def did_save(ls: LanguageServer, params: DidSaveTextDocumentParams):
    if ls.workspace.root_path:
        manager.update_local_index(ls.workspace.root_path)
    Validator.validate(ls, params.text_document.uri, manager)

@server.feature(TEXT_DOCUMENT_DEFINITION)
def definition(ls: LanguageServer, params: DefinitionParams):
    doc = ls.workspace.get_document(params.text_document.uri)
    line = doc.source.splitlines()[params.position.line]
    words = re.finditer(r'[a-zA-Z0-9_]+', line)
    for match in words:
        if match.start() <= params.position.character <= match.end():
            loc = manager.get_definition(match.group())
            if loc:
                return loc
    return None

@server.feature(TEXT_DOCUMENT_REFERENCES)
def references(ls: LanguageServer, params: ReferenceParams) -> List[Location]:
    doc = ls.workspace.get_document(params.text_document.uri)
    line = doc.source.splitlines()[params.position.line]
    words = re.finditer(r'[a-zA-Z0-9_]+', line)
    for match in words:
        if match.start() <= params.position.character <= match.end():
            symbol = match.group()
            return manager.get_references(symbol)
    return []

@server.feature(TEXT_DOCUMENT_HOVER)
def hover(ls: LanguageServer, params: HoverParams) -> Optional[Hover]:
    doc = ls.workspace.get_document(params.text_document.uri)
    line = doc.source.splitlines()[params.position.line]
    words = re.finditer(r'[a-zA-Z0-9_]+', line)
    for match in words:
        if match.start() <= params.position.character <= match.end():
            func_name = match.group()
            func_data = manager.find_function(func_name)
            if func_data:
                contents = MarkupContent(
                    kind=MarkupKind.Markdown,
                    value=f"**{func_name}{func_data['args_str']}**\n\n---\n\n{func_data['doc']}"
                )
                return Hover(contents=contents)
    return None

@server.feature(
    TEXT_DOCUMENT_SIGNATURE_HELP,
    SignatureHelpOptions(trigger_characters=['(', ','])
)
def signature_help(ls: LanguageServer, params: SignatureHelpParams) -> Optional[SignatureHelp]:
    doc = ls.workspace.get_document(params.text_document.uri)
    line = doc.source.splitlines()[params.position.line]
    before_cursor = line[:params.position.character]
    match = re.search(r'([a-zA-Z0-9_]+)\s*\([^()]*$', before_cursor)
    if not match: return None
    func_name = match.group(1)
    func_data = manager.find_function(func_name)
    if not func_data: return None
    params_content = before_cursor[match.end(1):].strip()
    active_param = params_content.count(',')
    sig_info = SignatureInformation(
        label=f"{func_name}{func_data['args_str']}",
        documentation=func_data['doc'],
        parameters=[ParameterInformation(label=p) for p in func_data['params']]
    )
    return SignatureHelp(signatures=[sig_info], active_signature=0, active_parameter=active_param)

@server.feature(TEXT_DOCUMENT_COMPLETION)
def completions(params: CompletionParams) -> CompletionList:
    items = manager.get_completions()
    return CompletionList(is_incomplete=False, items=items)

if __name__ == "__main__":
    server.start_io()