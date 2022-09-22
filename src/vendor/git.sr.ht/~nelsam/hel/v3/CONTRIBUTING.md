Contributing to hel
-------------------

## Getting Started

First, make sure you branch off of `master` and _do not_ use go modules.  I think
modules solved some problems but created a whole lot of others, so while I am
absolutely happy to support them for releases, the `master` branch will always
support standard GOPATH mode.

## Developing in Modules Mode

If you really don't want to develop hel in `GOPATH` mode, you can make your
changes in the latest version branch.  We keep a branch for each major version
which stores all of the tagged commits for that version - the latest commit should
be the latest minor/patch version for that major version.

Once you've made your changes in the version branch, copy those changes over to
`master` and make sure you run `goimports` to fix any of the dumb versioned import
paths.

## Tests

I highly, highly recommend that you write a test that fails against the current
`hel` master, first.  Whether you are implementing a new feature or fixing a bug,
it's important to have a test that will fail if the feature is reverted.

Typically, you will start in the `mock` package, which contains all of the
logic that creates mocks.  Take a look at the tests there and create a new test
that fails against the latest and greatest `hel`.

If the `mock` package doesn't have enough information to solve your issue, you
may need to descend into the `pkg` and `typ` packages, which handle parsing and
AST logic.

## Making Your PR

Create a PR against master, without go modules.  When I merge, I will cherry-pick
the changes on to any version branches that I think require it.  Generally, this
will only be the latest major version - but I will backport any fixes that seem
critically important and don't threaten backwards compatibility in old versions.

### Requesting Backports

If you are actively using an older version of `hel` and specifically would like
code backported to an older version, please let me know in the PR.  I will do
what I can.
