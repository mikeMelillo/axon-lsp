import pytest
import os
from server.axon_lsp.server import TrioParser, FantomParser


class TestTrioParser:
    def test_parse_basic_function(self):
        source = """name: myFunc
func
doc: "A test function"
src:
    (a, b) => do
        result: add(a, b)
    end
"""
        import tempfile

        with tempfile.NamedTemporaryFile(mode="w", suffix=".trio", delete=False) as f:
            f.write(source)
            f.flush()
            try:
                result = TrioParser.parse_file(f.name)
                assert "myFunc" in result
                assert result["myFunc"]["name"] == "myFunc"
                assert "test function" in result["myFunc"]["doc"]
                assert result["myFunc"]["params"] == ["a", "b"]
            finally:
                os.unlink(f.name)

    def test_parse_multiple_records(self):
        source = """name: first
func
doc: "First"
src:
    (a) => do end
---
name: second
func
doc: "Second"
src:
    (b) => do end
"""
        import tempfile

        with tempfile.NamedTemporaryFile(mode="w", suffix=".trio", delete=False) as f:
            f.write(source)
            f.flush()
            try:
                result = TrioParser.parse_file(f.name)
                assert "first" in result
                assert "second" in result
            finally:
                os.unlink(f.name)

    def test_parses_defcomp_syntax(self):
        source = """name: myComponent
func
doc: "Component"
src:
    defcomp myComponent(target) do
        out: someFunc(target)
        damp: readonly
    end
"""
        import tempfile

        with tempfile.NamedTemporaryFile(mode="w", suffix=".trio", delete=False) as f:
            f.write(source)
            f.flush()
            try:
                result = TrioParser.parse_file(f.name)
                assert "myComponent" in result
            finally:
                os.unlink(f.name)

    def test_extracts_doc_from_summary_tag(self):
        source = """name: myFunc
func
summary: "Summary doc"
src:
    (a) => do end
"""
        import tempfile

        with tempfile.NamedTemporaryFile(mode="w", suffix=".trio", delete=False) as f:
            f.write(source)
            f.flush()
            try:
                result = TrioParser.parse_file(f.name)
                assert "Summary doc" in result["myFunc"]["doc"]
            finally:
                os.unlink(f.name)


class TestFantomParser:
    def test_parse_axon_decorator(self):
        source = """@Axon
static Dict myFunc(Dict arg1, Int arg2) {
    // Some doc
}
"""
        import tempfile

        with tempfile.NamedTemporaryFile(mode="w", suffix=".fan", delete=False) as f:
            f.write(source)
            f.flush()
            try:
                result = FantomParser.parse_file(f.name)
                assert "myFunc" in result
                assert result["myFunc"]["name"] == "myFunc"
                assert "arg1" in result["myFunc"]["params"]
                assert "arg2" in result["myFunc"]["params"]
            finally:
                os.unlink(f.name)

    def test_handles_no_doc(self):
        source = """@Axon
static Void myFunc(Dict arg) {
}
"""
        import tempfile

        with tempfile.NamedTemporaryFile(mode="w", suffix=".fan", delete=False) as f:
            f.write(source)
            f.flush()
            try:
                result = FantomParser.parse_file(f.name)
                assert "myFunc" in result
                assert result["myFunc"]["doc"] == "No documentation available."
            finally:
                os.unlink(f.name)
