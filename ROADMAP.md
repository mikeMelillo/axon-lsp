# Axon LSP Roadmap

## Version 0.1.x - Current (POC)

### Completed
- Basic autocomplete for Axon functions
- Hover information
- Go to definition
- Diagnostics for undefined functions
- Support for `defcomp` syntax
- Basic syntax highlighting

### Known Limitations
- Core functions lack complete type hints
- Limited to single workspace directory
- No support for multiple reference directories

## Future Versions

### Version 0.2.0 - Enhanced Reference Support
- [ ] Multiple reference directories support:
  - Local Haxall repository
  - Cached core functions (built-in backstop)
  - Active working directory
- [ ] Improved type hints for core functions

### Version 0.3.0 - Advanced Features
- [ ] Signature help (parameter hints when typing function calls)
- [ ] Find references
- [ ] Rename symbol

### Version 1.0.0 - Production Ready
- [ ] Complete type hints for all core functions
- [ ] Full Haxall 4.0 support
- [ ] Marketplace publication
- [ ] Comprehensive test coverage

## Architecture Notes

### Current (Haxall 3.1.12)
- Indexes `.fan` files with `@Axon` annotations
- Indexes `.trio` files with `func` records
- Provides backstop from exported core functions

### Future (Haxall 4.0)
- Architecture changes expected
- Will need to adapt indexing logic
- May require different function discovery mechanism
