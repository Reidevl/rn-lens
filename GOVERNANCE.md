# Governance

Lens is a small project with one maintainer. This page says who can do what,
and how to get more access.

## Roles

| Role             | Who                                    | Can                                                            |
| ---------------- | -------------------------------------- | -------------------------------------------------------------- |
| **Contributor**  | Anyone                                 | Open issues, answer questions, join discussions                |
| **Collaborator** | People the maintainer invites          | Everything above, plus push branches and open PRs              |
| **Maintainer**   | [@Reidevl](https://github.com/Reidevl) | Review and merge PRs, release, invite and remove collaborators |

Pull requests are open to collaborators only. If you are not a collaborator
yet, start with an issue: a clear bug report or a well-argued proposal is the
most valuable contribution there is.

## Becoming a collaborator

There is no application form. The maintainer invites people who already help,
usually after about **three useful contributions over two months**, such as:

- Bug reports that are accurate and reproducible (steps, versions, logs).
- Proposals that were discussed and accepted.
- Helpful answers to other users' issues.

Collaborators must have
[two-factor authentication](https://docs.github.com/authentication/securing-your-account-with-two-factor-authentication-2fa)
enabled on their GitHub account.

## What collaborators agree to

- Follow [CONTRIBUTING.md](CONTRIBUTING.md): one change per PR with its tests
  and docs, Conventional Commits, about 400 changed lines per PR.
- Never push directly to `main` or force-push a shared branch. `main` changes
  only through pull requests.
- Do not merge pull requests, including your own: the maintainer reviews and
  merges every PR. GitHub gives collaborators write access, so this is an
  agreement, and the `main` ruleset enforces it as far as GitHub allows.
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md).
- Report vulnerabilities privately, as [SECURITY.md](SECURITY.md) describes.

## Leaving

Access is removed, with thanks, after **six months without activity**, or at
the collaborator's request. Anyone who left can be invited again.

The maintainer can remove access at any time for a breach of the Code of
Conduct or of the rules above.

## Decisions

The maintainer decides, after discussion in the issue or pull request.
Architecture decisions are recorded as ADRs in [`docs/adr`](docs/adr).

## Changing this document

Changes to this page go through a pull request like any other change.
