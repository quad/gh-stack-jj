# gh-stack-jj

Create stacked GitHub pull requests from [jj](https://jj-vcs.dev/) bookmarks. Each pushed bookmark in the current bookmark's ancestry becomes one pull request. A bookmark may contain one or more commits.

Existing pull requests are reused. The command does not create, rename, or push bookmarks.

## Install

As a GitHub CLI extension:

```sh
gh extension install quad/gh-stack-jj
```

Or build locally:

```sh
mise build
./gh-stack-jj --help
```

## Use

Push two or more bookmarks, then run:

```sh
gh stack-jj
```

(By default, it uses the nearest bookmark reachable from `@`; use `--bookmark NAME` to override it.) Fetch first when the remote-tracking bookmarks are out of date.

The command uses GitHub's [Stack API](https://github.com/github/gh-stack) when the stack has at least two pull requests. GitHub CLI authentication is used for all pull-request and stack operations.

## Development

The Go toolchain is pinned in `mise.toml`:

```sh
mise build
```

This formats, tests, vets, and builds the binary.
