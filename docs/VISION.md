# Jamsesh

Jamsesh is a collaboration substrate where a small team of humans drive Claude
Code instances against a shared git-backed session, producing artifacts together
in a live jam.

## The problem

When two or more humans want to use AI agents to collaborate on the same
work — design docs, specs, code — they hit the same walls. Each agent runs in
its own context with no awareness of what the others are doing. Each human has
to manually share state, summarize what their agent produced, and reconcile
differences. The collaborative loop is slow, and the version-control story is
invisible to the agents.

Tools that try to solve this either build their own version control (brittle,
fights git) or route everything through one shared agent (loses the individual
driver-pair relationship that makes agent work productive).

## The insight

Use real git as the substrate. Every change is a real commit. Every line of
exploration is a real branch. The portal is a thin social layer on top —
comments, presence, addressing metadata, live tree visualization — that
observes git and never tries to replace it.

Each human-agent pair gets their own namespace of refs to push to. A
server-side auto-merger continuously integrates non-conflicting work into a
shared draft ref, making the artifact converge live. Conflicts surface as
structured events that agents can act on in-session, mediated by their
human's prompts. When the jam is done, reconciliation happens locally in
the human's source-repo checkout with their own Claude Code agent. The
portal never touches the source repo.

## What you get

- A shared session where everyone's work is visible the moment it's pushed
- A live converged draft of the artifact you're producing
- Per-agent isolation when you want to explore something risky off-trunk
- A session goal that aligns every agent that joins
- Inline comments and addressed signals that flow into agent context each turn
- Conflict resolution that happens as the work is happening, not as an
  end-of-session ordeal
- A finalized branch you push to your source repo on your own terms
- Full recovery from any failure via `git fetch`
- An optional zero-friction playground mode for first contact — ephemeral,
  anonymous sessions that need no account and no org, so a prospective team
  can spin up a real jam and feel the substrate before committing to setup

## What it isn't

Jamsesh isn't a git forge, a code review tool, a real-time text editor, or a
replacement for your source repo. It isn't a SaaS-only product. And it isn't an
agent-to-agent backchannel — every signal between agents is visible to everyone
in the session.

## Who it's for

Small teams (2–5 humans) running Claude Code together on shared artifacts.
Doc-writing jams, spec refinement, design exploration, focused code refactors.
Teams that already work in git and want their AI collaboration to look like the
rest of their work — diff-able, recoverable, attributable.

The playground mode targets the same audience earlier in their evaluation
arc — prospective teams who want to feel a real multi-agent jam before
standing up a portal, OAuth provider, or org. The playground is a sample,
not a separate product; the durable substrate is where the real work lives.
