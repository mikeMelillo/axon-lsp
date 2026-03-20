# Axon LSP Roadmap

## Version 0.1.7 - Current (POC)

### Completed
- Basic autocomplete for Axon functions
- Hover information
- Go to definition (disregarding haxall repo links)
- Diagnostics for undefined functions
- Support for `defcomp` syntax
- Basic syntax highlighting

### Known Limitations
- Core functions lack complete type hints
- I'd like to index the core / skyspark functions completely for argument + type hints
- Need to decide on linking to haxall functions (external to github, local repo, etc.)
- Add support for multiple workspace directories
- Convert to 4.0 Xeto Spec Definitions for future versions

## Architecture Notes

### Current (Haxall 3.1.12)
- Indexes `.fan` files with `@Axon` annotations
- Indexes `.trio` files with `func` records
- Provides backstop from exported core functions

### Future (Haxall 4.0)
- Architecture changes expected
- Will need to adapt indexing logic
- May require different function discovery mechanism


### Patch Notes

- 0.1.5 - 0.1.7
    - Add //lspignore for single line omissions
    - Correct local fantom function indexing
    - Now omits "Text(args)" in strings correctly
    - Add detection for local helper functions