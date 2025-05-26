<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://github.com/anttikivi/reginald/blob/main/.github/reginald-gray-suit.svg?raw=true">
    <source media="(prefers-color-scheme: light)" srcset="https://github.com/anttikivi/reginald/blob/main/.github/reginald-black-suit.svg?raw=true">
    <img alt="Reginald the gopher valet" src="https://github.com/anttikivi/reginald/blob/main/.github/reginald-black-suit.svg?raw=true" width="200" style="max-width: 100%;">
  </picture>
</p>

<h1 align="center">Reginald</h1>

<div align="center">

👔 the personal workstation valet

[![CI](https://github.com/anttikivi/reginald/actions/workflows/ci.yml/badge.svg)](https://github.com/anttikivi/reginald/actions/workflows/ci.yml)

</div>

<!-- prettier-ignore-start -->
> [!NOTE]
> This project is still in early development. More info on the project will be
> added later and the current features don’t just yet match this README.
<!-- prettier-ignore-end -->

As a developer, I have tried to find a satisfactory way to manage my dotfiles
and have a workstation set up so that, ideally, I can get a new machine up and
running with a single command. However, I haven’t found any existing tool that
would fill these needs: they may not easily support all of the required
features, you might need to install some runtime (for example Python) before you
can run them, or you have to follow their bespoke workflow in order to use the
tool effectively. I find that these traits get in your way when you really just
want to have symbolic links to your dotfiles in a Git repository, install the
tools you need, and have a free hand to extend the tool’s workflow in the way
you want to. Also, using Bash scripts for this get convoluted as the steps to
run and the requirements grow. Ansible and Nix might be too involved for your
needs.

Reginald uses a simple, single config file to define a set of idempotent,
system-wide tasks describing how your system should look. By default, it can set
up links to your dotfiles in the directory of your choosing and install tools
using your system’s (or some other) package manager.

Even though Reginald is splendid already as is (see the picture of Reginald at
the start of the README), they can always do more. That’s why Reginald has a
language-agnostic plugin system. You can use it to teach Reginald to do
effectively anything. Just install the plugins you need and use the same config
file to configure them.

## License

Reginald is licensed under the MIT License. See the [LICENSE](LICENSE) file for
more information.

Reginald the gopher valet is based on the Go gopher. Reginald the gopher valet
is licensed under the
[Creative Commons Attribution 4.0 International license](https://creativecommons.org/licenses/by/4.0/).
The Go gopher is designed by Renee French and licensed under the
[Creative Commons 3.0 Attributions license](https://creativecommons.org/licenses/by/3.0/deed.en).
The [vector data](https://github.com/golang-samples/gopher-vector) used for the
Go gopher is by Takuya Ueda and licensed under the
[Creative Commons 3.0 Attributions license](https://creativecommons.org/licenses/by/3.0/deed.en).
