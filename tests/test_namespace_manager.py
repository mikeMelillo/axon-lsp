import pytest
from unittest.mock import MagicMock
from lsprotocol.types import Location, Position, Range
from server.axon_lsp.server import NamespaceManager, TrioParser


class TestNamespaceManagerCore:
    """Tests for core function loading from cache."""

    def test_loads_core_functions_from_cache(self):
        mgr = NamespaceManager()
        assert len(mgr.core_funcs) > 0, "Should load functions from cache"
        assert "hisRead" in mgr.core_funcs, "Expecting hisRead in cache"

    def test_get_definition_finds_core_function(self):
        mgr = NamespaceManager()
        result = mgr.get_definition("read")
        assert result is not None

    def test_find_function_returns_core_function_dict(self):
        mgr = NamespaceManager()
        result = mgr.find_function("read")
        assert result is not None
        assert result["name"] == "read"
        assert "doc" in result

    def test_completions_include_core_functions(self):
        mgr = NamespaceManager()
        completions = mgr.get_completions()
        assert len(completions) > 0
        assert any(c.label == "read" for c in completions)


class TestNamespaceManagerLocal:
    """Tests for local function handling."""

    def test_loads_local_functions_from_file(self, mgr_with_local_funcs):
        local_funcs = mgr_with_local_funcs.local_funcs
        assert len(local_funcs) > 0, "Should have loaded functions from file"
        assert "bananaPhone" in local_funcs, "Expecting bananaPhone from test file"

    def test_get_definition_finds_local_function(self, mgr_with_local_funcs):
        result = mgr_with_local_funcs.get_definition("bananaPhone")
        assert result is not None

    def test_find_function_returns_local_function(self, mgr_with_local_funcs):
        result = mgr_with_local_funcs.find_function("bananaPhone")
        assert result is not None
        assert result["name"] == "bananaPhone"
        assert "doc" in result

    def test_local_functions_in_completions(self, mgr_with_local_funcs):
        completions = mgr_with_local_funcs.get_completions()
        labels = [c.label for c in completions]
        assert "bananaPhone" in labels, "Local functions should appear in completions"


class TestNamespaceManagerPriority:
    """Tests for function lookup priority (local > external > core)."""

    def test_local_funcs_take_precedence_over_core(self, mgr_with_local_funcs):
        mgr_with_local_funcs.core_funcs["myLocalTest"] = {
            "name": "myLocalTest",
            "doc": "Core version",
            "kind": 1,
            "location": None,
        }
        mgr_with_local_funcs.local_funcs["myLocalTest"] = {
            "name": "myLocalTest",
            "doc": "Local version",
            "kind": 1,
            "location": None,
        }
        result = mgr_with_local_funcs.find_function("myLocalTest")
        assert result["doc"] == "Local version"

    def test_external_funcs_take_precedence_over_core(self):
        mgr = NamespaceManager()
        mgr.external_funcs = {
            "read": {
                "name": "read",
                "doc": "External read",
                "kind": 1,
                "location": None,
            }
        }
        result = mgr.find_function("read")
        assert result["doc"] == "External read"

    def test_local_takes_precedence_over_external(self):
        mgr = NamespaceManager()
        mgr.external_funcs = {
            "myFunc": {
                "name": "myFunc",
                "doc": "External",
                "kind": 1,
                "location": None,
            }
        }
        mgr.local_funcs = {
            "myFunc": {
                "name": "myFunc",
                "doc": "Local",
                "kind": 1,
                "location": None,
            }
        }
        result = mgr.find_function("myFunc")
        assert result["doc"] == "Local"


class TestNamespaceManagerReferences:
    """Tests for reference tracking."""

    def test_add_reference_tracks_location(self):
        mgr = NamespaceManager()
        loc = Location(
            uri="file:///test.trio",
            range=Range(
                start=Position(line=0, character=0),
                end=Position(line=0, character=10),
            ),
        )
        mgr.add_reference("myFunc", loc)
        assert "myFunc" in mgr.references_map
        assert loc in mgr.references_map["myFunc"]

    def test_clear_references_for_uri(self):
        mgr = NamespaceManager()
        loc1 = Location(
            uri="file:///test1.trio",
            range=Range(
                start=Position(line=0, character=0),
                end=Position(line=0, character=10),
            ),
        )
        loc2 = Location(
            uri="file:///test2.trio",
            range=Range(
                start=Position(line=0, character=0),
                end=Position(line=0, character=10),
            ),
        )
        mgr.add_reference("myFunc", loc1)
        mgr.add_reference("myFunc", loc2)
        assert len(mgr.references_map["myFunc"]) == 2

        mgr.clear_references_for_uri("file:///test1.trio")
        assert len(mgr.references_map["myFunc"]) == 1
        assert loc2 in mgr.references_map["myFunc"]
