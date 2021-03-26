# Upspeak

Cloud-native, contextual reader, writer, archiver, and annotator built for individuals. Lets users connect multiple web-based sources into self-updating book like interfaces. Wants to work with Discourse, Matrix, and GitHub Issues.

![Project status: Experimental](https://img.shields.io/badge/status-experimental-red) [![Chat on Matrix at +upspeak@matrix.org](https://img.shields.io/matrix/upspeak:matrix.org)](https://app.element.io/#/room/#upspeak:matrix.org)

## Goal

Upspeak aims to provide simple tools to build information archives, aka, repositories. These repositories can gather data from (or, proxy for us in) places on the web, practically any system accessible over HTTP. It will then let us annotate the data, and (if we want it to) send replies back, all within their contexts.

It will integrate with tools that we already use to consume or create information in multiple (often shared) contexts. It will be able to replicate continuously from and/or to places like, with varying degrees of capability:

* *IM Networks*: Matrix
* *Issue queues*: GitHub Issues, GitLab Issues, Bugzilla
* *Forums*: Discourse, Reddit
* *HTTP APIs*: RSS/Atom Feeds, Webhooks

Think of Upspeak as a personal library of self-updating books, each representing an archive of online publications, or happenings of online communities you (may) (want to) participate in.

> Editor’s note: Upspeak has matured as a thought over the years through close observation of how services like GitHub, Kindle, and Bear take cognitive load away.]

## Data ownership

Every user owns and controls their own data. Upspeak behaves like a domain-specific user-agent for accessing and archiving parts of the ephemeral web.

Upspeak will provide the right balance of tools to run it locally, as well as, deploy to a cloud provider.

This idea of data ownership is contained in the concept of _“Repository”_. A repository, for Upspeak, is a collection of documents, which you can mutate manually, &/:

* configure sources
* specify filters
* specify formats, or how, you want to see entries
* configure destinations — places where this data will get pushed to
* schedule rate of update — can be realtime, depends on the sources and destinations.

## In short

Upspeak will be a client for Matrix, Discourse, feeds, blogs, and wikis, with which users can save their data where they want.

It will try to blend synchronous and asynchronous communication modes, and a focus on distraction-free experience.

## Further reading

- [On context][_context]
- [Focus areas][_focus]
- [Hypothesis][_hypo]
- [Data model][_model]

## Dev

- Install `libssl-dev` on Ubuntu

## License

[Apache-2.0](https://www.apache.org/licenses/LICENSE-2.0)

[_context]:https://www.upspeak.net/about/context.html
[_focus]:https://www.upspeak.net/about/focus-areas.html
[_hypo]:https://www.upspeak.net/about/hypothesis.html
[_model]:https://www.upspeak.net/about/data-model.html
