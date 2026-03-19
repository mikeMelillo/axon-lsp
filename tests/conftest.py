import pytest
import sys
import os
from unittest.mock import MagicMock

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))


@pytest.fixture
def fixtures_path():
    return os.path.join(os.path.dirname(__file__), "fixtures")


@pytest.fixture
def sample_trio_content():
    return """(a, b) => do
    result: add(a, b)
end
"""


@pytest.fixture
def trio_with_string_literals():
    return """data: [
    {item: 123, funcName: "dontCatchMe(hello)"},
    {item: 456, funcName: "alsoShouldNotMatch(x, y)"},
]
"""


@pytest.fixture
def trio_with_function_params():
    return """definer: (inputFunction, defName) => inputFunction(defName)
map(callback, items)
"""


@pytest.fixture
def trio_with_comments():
    return """name: myFunc
func
doc: "Test"
src:
    result: calculate(x) // this comment "shouldNotMatch(y)"
"""


@pytest.fixture
def trio_multiline_src():
    return """name: myCalc
func
doc: "A calculation"
src:
    (a, b, c) => do
        first: processA(a)
        second: processB(first, b)
        result: combine(second, c)
    end
"""


@pytest.fixture
def trio_indented_functions():
    return """name: parentFunc
func
doc: "Parent"
src:
    (outer) => do
        helper: (x) => process(x)
        nested: helper(outer)
    end
"""


@pytest.fixture
def mock_ls():
    ls = MagicMock()
    ls.workspace.root_path = "/fake/path"
    return ls


@pytest.fixture
def mock_text_document():
    doc = MagicMock()
    doc.source = ""
    return doc


@pytest.fixture
def mgr_with_local_funcs():
    """NamespaceManager with local functions loaded from test/file.trio."""
    from server.axon_lsp.server import NamespaceManager, TrioParser

    mgr = NamespaceManager()
    test_file = os.path.join(os.path.dirname(__file__), "..", "test", "file.trio")
    mgr.local_funcs = TrioParser.parse_file(test_file)
    return mgr
