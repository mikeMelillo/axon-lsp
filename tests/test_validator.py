import pytest
from unittest.mock import MagicMock
from server.axon_lsp.server import Validator, NamespaceManager


class TestParseLocalFunctions:
    def test_detects_basic_function_definition(self):
        source = """helper: (x) => process(x)
otherFunc: (a, b) => do end
"""
        local_funcs, param_scopes = Validator._parse_local_functions(source)
        assert "helper" in local_funcs
        assert "otherFunc" in local_funcs

    def test_ignores_string_literals(self, trio_with_string_literals):
        local_funcs, param_scopes = Validator._parse_local_functions(
            trio_with_string_literals
        )
        assert "dontCatchMe" not in local_funcs
        assert "alsoShouldNotMatch" not in local_funcs

    def test_ignores_comments(self, trio_with_comments):
        local_funcs, param_scopes = Validator._parse_local_functions(trio_with_comments)
        assert "shouldNotMatch" not in local_funcs

    def test_parses_function_parameters(self, trio_with_function_params):
        local_funcs, param_scopes = Validator._parse_local_functions(
            trio_with_function_params
        )
        assert "definer" in local_funcs
        assert "inputFunction" in param_scopes[0]
        assert "defName" in param_scopes[0]

    def test_parameters_valid_within_function_scope(self):
        source = """wrapper: (callback, data) => do
    result: callback(data)
    another: process(result)
end
"""
        local_funcs, param_scopes = Validator._parse_local_functions(source)
        line_0_params = param_scopes.get(0, set())
        line_1_params = param_scopes.get(1, set())
        assert "callback" in line_0_params
        assert "data" in line_0_params
        assert "callback" in line_1_params
        assert "data" in line_1_params

    def test_scope_ends_at_less_indentation(self):
        source = """helper: (x) => inner(x)
wrapper: (a) => do
    something: a
end
"""
        local_funcs, param_scopes = Validator._parse_local_functions(source)
        assert "helper" in local_funcs
        assert "wrapper" in local_funcs

    def test_ignores_dict_keys_in_multiline_dict(self):
        source = """object: {
    data: 'foo',
    name: 'bar',
}
result: process(object)
"""
        local_funcs, param_scopes = Validator._parse_local_functions(source)
        assert "object" in local_funcs
        assert "result" in local_funcs
        assert "data" not in local_funcs
        assert "name" not in local_funcs

    def test_nested_dicts_ignored(self):
        source = """outer: {
    inner: {
        key: 'value',
    },
}
anotherVar: foo()
"""
        local_funcs, param_scopes = Validator._parse_local_functions(source)
        assert "outer" in local_funcs
        assert "anotherVar" in local_funcs
        assert "inner" not in local_funcs
        assert "key" not in local_funcs

    def test_func_with_quotes_in_expression(self):
        source = """getTotal: (pt, ds) => pt.hisRead(ds).foldCol("v0", sum)
result: getTotal(point, dates)
"""
        local_funcs, param_scopes = Validator._parse_local_functions(source)
        assert "getTotal" in local_funcs
        assert "result" in local_funcs


class TestValidate:
    def test_reports_undefined_function(self, mock_ls, mock_text_document):
        source = "undefinedFunc(x)"
        mock_ls.workspace.get_text_document.return_value = mock_text_document
        mock_text_document.source = source

        mgr = NamespaceManager()
        Validator.validate(mock_ls, "file:///test.trio", mgr)

        mock_ls.publish_diagnostics.assert_called()
        call_args = mock_ls.publish_diagnostics.call_args
        diagnostics = call_args[0][1]
        assert any(
            d.message == "Undefined function: undefinedFunc" for d in diagnostics
        )

    def test_ignores_defined_local_function(self, mock_ls, mock_text_document):
        source = """name: myFunc
func
doc: "My function"
src:
    myFunc(x)
"""
        mock_ls.workspace.get_text_document.return_value = mock_text_document
        mock_text_document.source = source

        mgr = NamespaceManager()
        mgr.update_local_index = MagicMock()
        mgr.local_funcs = {
            "myFunc": {
                "name": "myFunc",
                "doc": "My function",
                "kind": 1,
                "location": None,
            }
        }
        Validator.validate(mock_ls, "file:///test.trio", mgr)

        call_args = mock_ls.publish_diagnostics.call_args
        diagnostics = call_args[0][1]
        assert not any(d.message.startswith("Undefined function:") for d in diagnostics)

    def test_ignores_function_parameters(self, mock_ls, mock_text_document):
        source = """name: tester
func
src:
    definer: (inputFunction, defName) => inputFunction(defName)
"""
        mock_ls.workspace.get_text_document.return_value = mock_text_document
        mock_text_document.source = source

        mgr = NamespaceManager()
        mgr.update_local_index = MagicMock()
        mgr.local_funcs = {}
        Validator.validate(mock_ls, "file:///test.trio", mgr)

        call_args = mock_ls.publish_diagnostics.call_args
        diagnostics = call_args[0][1]
        assert not any(
            d.message == "Undefined function: inputFunction" for d in diagnostics
        )
        assert not any(d.message == "Undefined function: defName" for d in diagnostics)

    def test_ignores_strings_with_func_patterns(self, mock_ls, mock_text_document):
        source = """name: testFunc
func
src:
    data: {funcName: "dontCatchMe(hello)"}
"""
        mock_ls.workspace.get_text_document.return_value = mock_text_document
        mock_text_document.source = source

        mgr = NamespaceManager()
        mgr.update_local_index = MagicMock()
        mgr.local_funcs = {}
        Validator.validate(mock_ls, "file:///test.trio", mgr)

        call_args = mock_ls.publish_diagnostics.call_args
        diagnostics = call_args[0][1]
        assert not any(
            d.message == "Undefined function: dontCatchMe" for d in diagnostics
        )

    def test_ignores_keywords(self, mock_ls, mock_text_document):
        source = """name: myFunc
func
src:
    if(x)
    do()
    return(result)
"""
        mock_ls.workspace.get_text_document.return_value = mock_text_document
        mock_text_document.source = source

        mgr = NamespaceManager()
        mgr.update_local_index = MagicMock()
        mgr.local_funcs = {}
        Validator.validate(mock_ls, "file:///test.trio", mgr)

        call_args = mock_ls.publish_diagnostics.call_args
        diagnostics = call_args[0][1]
        undefined_messages = [
            d.message for d in diagnostics if d.message.startswith("Undefined")
        ]
        assert len(undefined_messages) == 0

    def test_ignores_line_with_lspignore(self, mock_ls, mock_text_document):
        source = "undefinedFunc(x) //lspignore"
        mock_ls.workspace.get_text_document.return_value = mock_text_document
        mock_text_document.source = source

        mgr = NamespaceManager()
        Validator.validate(mock_ls, "file:///test.trio", mgr)

        call_args = mock_ls.publish_diagnostics.call_args
        diagnostics = call_args[0][1]
        assert len(diagnostics) == 0

    def test_lspignore_is_case_sensitive(self, mock_ls, mock_text_document):
        source = "undefinedFunc(x) //LspIgnore"
        mock_ls.workspace.get_text_document.return_value = mock_text_document
        mock_text_document.source = source

        mgr = NamespaceManager()
        Validator.validate(mock_ls, "file:///test.trio", mgr)

        call_args = mock_ls.publish_diagnostics.call_args
        diagnostics = call_args[0][1]
        assert any(
            d.message == "Undefined function: undefinedFunc" for d in diagnostics
        )

    def test_lspignore_with_multiple_funcs_on_line(self, mock_ls, mock_text_document):
        source = "funcA(x) funcB(y) //lspignore"
        mock_ls.workspace.get_text_document.return_value = mock_text_document
        mock_text_document.source = source

        mgr = NamespaceManager()
        Validator.validate(mock_ls, "file:///test.trio", mgr)

        call_args = mock_ls.publish_diagnostics.call_args
        diagnostics = call_args[0][1]
        assert len(diagnostics) == 0

    def test_without_lspignore_still_errors(self, mock_ls, mock_text_document):
        source = "undefinedFunc(x)"
        mock_ls.workspace.get_text_document.return_value = mock_text_document
        mock_text_document.source = source

        mgr = NamespaceManager()
        Validator.validate(mock_ls, "file:///test.trio", mgr)

        call_args = mock_ls.publish_diagnostics.call_args
        diagnostics = call_args[0][1]
        assert any(
            d.message == "Undefined function: undefinedFunc" for d in diagnostics
        )
