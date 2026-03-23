import pytest
from unittest.mock import MagicMock
from server.axon_lsp.server import NamespaceManager


class TestSignatureHelp:
    def test_build_signature_help_for_known_function(self):
        mgr = NamespaceManager()
        result = mgr.build_signature_help("read")
        assert result is not None
        assert len(result.signatures) == 1
        assert "read" in result.signatures[0].label

    def test_build_signature_help_returns_none_for_unknown(self):
        mgr = NamespaceManager()
        result = mgr.build_signature_help("nonexistentFunction123")
        assert result is None

    def test_signature_help_includes_parameters(self):
        mgr = NamespaceManager()
        result = mgr.build_signature_help("read")
        assert result is not None
        sig = result.signatures[0]
        assert len(sig.parameters) > 0

    def test_signature_help_includes_documentation(self):
        mgr = NamespaceManager()
        result = mgr.build_signature_help("read")
        assert result is not None
        assert result.signatures[0].documentation is not None

    def test_signature_help_returns_correct_active_signature_index(self):
        mgr = NamespaceManager()
        result = mgr.build_signature_help("read")
        assert result is not None
        assert result.active_signature == 0

    def test_signature_help_returns_correct_active_parameter_index(self):
        mgr = NamespaceManager()
        result = mgr.build_signature_help("read")
        assert result is not None
        assert result.active_parameter == 0

    def test_signature_help_for_hisRead(self):
        mgr = NamespaceManager()
        result = mgr.build_signature_help("hisRead")
        assert result is not None
        assert "hisRead" in result.signatures[0].label

    def test_signature_help_for_local_function(self, mgr_with_local_funcs):
        result = mgr_with_local_funcs.build_signature_help("bananaPhone")
        assert result is not None
        assert "bananaPhone" in result.signatures[0].label
