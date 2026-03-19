import pytest
from unittest.mock import MagicMock
from server.axon_lsp.server import NamespaceManager


class TestNamespaceManager:
    def test_loads_core_functions_from_cache(self):
        mgr = NamespaceManager()
        assert len(mgr.core_funcs) > 0, "Should load functions from cache"

    def test_get_definition_finds_core_function(self):
        mgr = NamespaceManager()
        mgr.update_local_index = MagicMock()
        result = mgr.get_definition("read")
        assert result is not None

    def test_find_function_returns_function_dict(self):
        mgr = NamespaceManager()
        mgr.update_local_index = MagicMock()
        result = mgr.find_function("read")
        assert result is not None
        assert "name" in result
        assert "doc" in result

    def test_completions_include_core_functions(self):
        mgr = NamespaceManager()
        mgr.update_local_index = MagicMock()
        completions = mgr.get_completions()
        assert len(completions) > 0
        assert any(c.label == "read" for c in completions)

    def test_add_reference_tracks_location(self):
        mgr = NamespaceManager()
        from lsprotocol.types import Location, Position, Range

        loc = Location(
            uri="file:///test.trio",
            range=Range(
                start=Position(line=0, character=0), end=Position(line=0, character=10)
            ),
        )
        mgr.add_reference("myFunc", loc)
        assert "myFunc" in mgr.references_map
        assert loc in mgr.references_map["myFunc"]

    def test_clear_references_for_uri(self):
        mgr = NamespaceManager()
        from lsprotocol.types import Location, Position, Range

        loc1 = Location(
            uri="file:///test1.trio",
            range=Range(
                start=Position(line=0, character=0), end=Position(line=0, character=10)
            ),
        )
        loc2 = Location(
            uri="file:///test2.trio",
            range=Range(
                start=Position(line=0, character=0), end=Position(line=0, character=10)
            ),
        )
        mgr.add_reference("myFunc", loc1)
        mgr.add_reference("myFunc", loc2)
        assert len(mgr.references_map["myFunc"]) == 2

        mgr.clear_references_for_uri("file:///test1.trio")
        assert len(mgr.references_map["myFunc"]) == 1
        assert loc2 in mgr.references_map["myFunc"]

    def test_external_funcs_take_precedence_over_core(self):
        mgr = NamespaceManager()
        mgr.update_local_index = MagicMock()
        mgr.external_funcs = {
            "read": {
                "name": "read",
                "doc": "Custom read",
                "kind": 1,
                "location": None,
            }
        }
        result = mgr.find_function("read")
        assert result["doc"] == "Custom read"

    def test_local_funcs_take_precedence_over_external(self):
        mgr = NamespaceManager()
        mgr.update_local_index = MagicMock()
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
