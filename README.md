<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://github.com/reginald-project/reginald/blob/main/.github/reginald-gray-suit.svg?raw=true">
    <source media="(prefers-color-scheme: light)" srcset="https://github.com/reginald-project/reginald/blob/main/.github/reginald-black-suit.svg?raw=true">
    <img alt="Reginald the gopher valet" src="https://github.com/reginald-project/reginald/blob/main/.github/reginald-black-suit.svg?raw=true" width="200" style="max-width: 100%;">
  </picture>
</p>

<h1 align="center">Reginald</h1>

<div align="center">

👔 the personal workstation valet

[![CI](https://github.com/reginald-project/reginald/actions/workflows/ci.yml/badge.svg)](https://github.com/reginald-project/reginald/actions/workflows/ci.yml)

</div>

<!-- prettier-ignore-start -->
> [!NOTE]
> This project is still in early development. More info on the project will be
> added later and the current features don’t just yet match this README.
<!-- prettier-ignore-end -->

As a developer, I have tried to find a reasonable way to manage my dotfiles and
have a workstation set up so that, ideally, I can get a new machine up and
running with a single command. However, I’ve found that doing this with existing
solutions can become a hassle as the existing tools might require you to install
a runtime (for example, Python) or adapt a workflow that seems too complicated
for the task at hand. Additionally, Bash scripts can become hard to maintain.

Reginald can help as the workstation valet. You need to write a single config
file telling Reginald what to do and it will take care of your setup. It creates
symbolic links for your dotfiles from the directory that you choose and installs
the packages you need.

Reginald can also learn new tasks. It has a language-agnostic plugin system that
you can use to add new subcommands and tasks to Reginald.

## Getting Started

As the project is in an early stage, there is no prebuilt binaries or releases
available. You can still build and run Reginald on your machine if you have Go
and preferably `make` installed.

After cloning the repository, change to it and run:

    make build

This builds Reginald as `./reginald` at the root of the repository.

## License

Copyright (c) 2025 The Reginald Authors

Reginald is licensed under the Apache-2.0 license. See the [LICENSE](LICENSE)
file for more information.

Reginald the gopher valet is licensed under the Creative Commons Attribution 4.0
International license. See the [CC-BY-4.0.txt](CC-BY-4.0.txt) file for more
information.

Reginald the gopher valet is based on the Go gopher. The Go gopher is designed
by Renee French and licensed under the Creative Commons 4.0 Attribution license.
The vector data used for the Go gopher is by Takuya Ueda and licensed under the
Creative Commons 3.0 Attributions license.

Please see the [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES) for full attribution
and license information.
