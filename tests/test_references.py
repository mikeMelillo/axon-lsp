import pytest
from lsprotocol.types import Location, Position, Range
from server.axon_lsp.server import NamespaceManager


class TestGetReferences:
    def test_get_references_returns_all_locations(self):
        mgr = NamespaceManager()
        loc = Location(
            uri="file:///test.trio",
            range=Range(
                start=Position(line=0, character=0),
                end=Position(line=0, character=10),
            ),
        )
        mgr.add_reference("myFunc", loc)
        refs = mgr.get_references("myFunc")
        assert len(refs) == 1
        assert refs[0] == loc

    def test_get_references_returns_empty_for_unknown(self):
        mgr = NamespaceManager()
        refs = mgr.get_references("nonexistentFunction123")
        assert refs == []

    def test_get_references_returns_multiple_locations(self):
        mgr = NamespaceManager()
        loc1 = Location(
            uri="file:///test1.trio",
            range=Range(
                start=Position(line=0, character=0), end=Position(line=0, character=5)
            ),
        )
        loc2 = Location(
            uri="file:///test2.trio",
            range=Range(
                start=Position(line=5, character=10), end=Position(line=5, character=15)
            ),
        )
        mgr.add_reference("myFunc", loc1)
        mgr.add_reference("myFunc", loc2)
        refs = mgr.get_references("myFunc")
        assert len(refs) == 2

    def test_get_references_different_symbols(self):
        mgr = NamespaceManager()
        loc1 = Location(
            uri="file:///test.trio",
            range=Range(
                start=Position(line=0, character=0), end=Position(line=0, character=5)
            ),
        )
        loc2 = Location(
            uri="file:///test.trio",
            range=Range(
                start=Position(line=2, character=0), end=Position(line=2, character=5)
            ),
        )
        mgr.add_reference("funcA", loc1)
        mgr.add_reference("funcB", loc2)
        assert len(mgr.get_references("funcA")) == 1
        assert len(mgr.get_references("funcB")) == 1
        assert mgr.get_references("funcA") != mgr.get_references("funcB")

    def test_get_references_core_function(self, mgr_with_local_funcs):
        refs = mgr_with_local_funcs.get_references("read")
        assert isinstance(refs, list)

    def test_get_references_local_function(self, mgr_with_local_funcs):
        refs = mgr_with_local_funcs.get_references("bananaPhone")
        assert isinstance(refs, list)
