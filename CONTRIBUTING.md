# How to contribute

We'd love to accept your patches and contributions to this project. There are
just a few small guidelines you need to follow.

## Contribution agreement

The project is distributed under the terms shown in the LICENSE file. 
By submitting a contribution, you agree to grant sufficient copyright and patent
licenses over your contribution, as required in order to enable the LICENSE to
be granted to others.

## Code reviews

We use GitHub pull requests for code reviews. Consult [GitHub Help] for more
information on using pull requests.

Some project members may merge changes without review, however, such changes
should be reasonably rare, and performed with care (mindful of breaking other
projects).

## Style guide

There are no style rules, beyond the use of `gofmt`, that are _strictly_ 
enforced (either through code reviews, or automation).

That said, code is generally expected to follow the [Google Go style guide], and
reviewers may uphold that during code reviews.

In addition to ensuring your code successfully builds, tests pass, and is
formatted with `gofmt`, we prefer to review 
[smaller, more focused pull requests][Small CLs], rather than large pull
requests containing multiple unrelated changes.

[GitHub Help]: https://help.github.com/articles/about-pull-requests/
[Google Go style guide]: https://google.github.io/styleguide/go/
[Small CLs]: https://google.github.io/eng-practices/review/developer/small-cls.html