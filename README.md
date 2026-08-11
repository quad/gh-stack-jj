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

## License

Vibecoded: CC0 1.0 Universal.
