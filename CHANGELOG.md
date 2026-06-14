# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `Node.nodesBetween`, `Node.textBetween`, `Fragment.nodesBetween`, and `Fragment.textBetween` methods.
- Support for `consuming`, `ignore`, `skip`, and `closeParent` in JSON schema.
- `ParseOptions.RuleFromNode` parse option.

### Changed

- `NewSchema` takes a `SchemaCallbacks` argument instead of a `validators` map.

## [0.1.0] - 2026-06-14

### Added

- Port of prosemirror-model, v1.25.8.

[unreleased]: https://gitlab.com/tozd/go/prosemirror/-/compare/v0.1.0...main
[0.1.0]: https://gitlab.com/tozd/go/prosemirror/-/tags/v0.1.0

<!-- markdownlint-disable-file MD024 -->
