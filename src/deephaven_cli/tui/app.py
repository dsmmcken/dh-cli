"""Management TUI for Deephaven CLI.

Launched by ``dh`` with no arguments. Provides:
- First-run wizard when no versions are installed (Java check + version picker + install)
- Main menu when versions are installed (REPL, serve, versions, servers, etc.)
"""
from __future__ import annotations

import subprocess
import sys

from textual.app import App, ComposeResult
from textual.containers import Center, Horizontal, Vertical
from textual.screen import Screen
from textual.widgets import (
    Button,
    Footer,
    Header,
    Label,
    ListItem,
    ListView,
    OptionList,
    ProgressBar,
    Static,
)
from textual.widgets.option_list import Option


# ---------------------------------------------------------------------------
# First-Run Wizard Screens
# ---------------------------------------------------------------------------


class WelcomeScreen(Screen):
    """Screen 1: Welcome splash."""

    CSS = """
    WelcomeScreen {
        align: center middle;
    }

    #welcome-box {
        width: 56;
        height: auto;
        border: double $primary;
        padding: 2 4;
    }

    #welcome-box Static {
        text-align: center;
        width: 100%;
    }

    #welcome-box Button {
        margin-top: 2;
    }
    """

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="welcome-box"):
                yield Static("[bold]Deephaven CLI[/bold]", id="title")
                yield Static("")
                yield Static("Welcome! Let's get your environment ready.")
                yield Static("")
                yield Static("This wizard will:")
                yield Static("  1. Check for Java (or install it)")
                yield Static("  2. Install a Deephaven engine version")
                yield Static("  3. Get you into a REPL")
                yield Static("")
                with Center():
                    yield Button("Get Started", variant="primary", id="btn-start")

    def on_button_pressed(self, event: Button.Pressed) -> None:
        if event.button.id == "btn-start":
            self.app.push_screen(JavaCheckScreen())


class JavaCheckScreen(Screen):
    """Screen 2: Java detection and optional install."""

    CSS = """
    JavaCheckScreen {
        align: center middle;
    }

    #java-box {
        width: 56;
        height: auto;
        border: double $primary;
        padding: 2 4;
    }

    #java-box Static {
        width: 100%;
    }

    #java-buttons {
        margin-top: 2;
    }

    #java-buttons Button {
        margin-right: 2;
    }
    """

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="java-box"):
                yield Static("[bold]Step 1 of 3 -- Java[/bold]")
                yield Static("")
                yield Static("Checking for Java 17+...", id="java-status")
                yield Static("", id="java-detail")
                with Horizontal(id="java-buttons"):
                    yield Button("Next", variant="primary", id="btn-next", disabled=True)
                    yield Button("Install Java", id="btn-install-java", disabled=True)
                    yield Button("Skip", id="btn-skip")

    def on_mount(self) -> None:
        self._check_java()

    def _check_java(self) -> None:
        from deephaven_cli.manager.java import detect_java

        status = self.query_one("#java-status", Static)
        detail = self.query_one("#java-detail", Static)
        btn_next = self.query_one("#btn-next", Button)
        btn_install = self.query_one("#btn-install-java", Button)

        info = detect_java()
        if info:
            status.update(f"[green]Java {info['version']} found[/green]")
            detail.update(f"  Path: {info['path']}\n  Source: {info['source']}")
            btn_next.disabled = False
            btn_install.disabled = True
        else:
            status.update("[red]No compatible Java found[/red]")
            detail.update(
                "Deephaven requires a JDK to run its engine.\n"
                "We can install Eclipse Temurin 21 (LTS) to ~/.dh/java/\n"
                "No sudo required."
            )
            btn_next.disabled = True
            btn_install.disabled = False

    def on_button_pressed(self, event: Button.Pressed) -> None:
        if event.button.id == "btn-next" or event.button.id == "btn-skip":
            self.app.push_screen(VersionPickerScreen())
        elif event.button.id == "btn-install-java":
            self._install_java()

    def _install_java(self) -> None:
        status = self.query_one("#java-status", Static)
        btn_install = self.query_one("#btn-install-java", Button)
        btn_install.disabled = True
        status.update("Installing Eclipse Temurin 21... (this may take a moment)")

        # Run install in a worker to keep TUI responsive
        self.run_worker(self._do_install_java, thread=True)

    async def _do_install_java(self) -> None:
        from deephaven_cli.manager.java import download_java

        try:
            download_java()
            self.app.call_from_thread(self._java_install_done, None)
        except Exception as e:
            self.app.call_from_thread(self._java_install_done, str(e))

    def _java_install_done(self, error: str | None) -> None:
        status = self.query_one("#java-status", Static)
        btn_next = self.query_one("#btn-next", Button)
        if error:
            status.update(f"[red]Install failed: {error}[/red]")
            btn_next.disabled = True
            btn_install = self.query_one("#btn-install-java", Button)
            btn_install.disabled = False
        else:
            status.update("[green]Java installed successfully![/green]")
            btn_next.disabled = False


class VersionPickerScreen(Screen):
    """Screen 3: Select a Deephaven version to install."""

    CSS = """
    VersionPickerScreen {
        align: center middle;
    }

    #version-box {
        width: 56;
        height: auto;
        max-height: 80%;
        border: double $primary;
        padding: 2 4;
    }

    #version-box Static {
        width: 100%;
    }

    #version-list {
        height: 10;
        margin: 1 0;
    }

    #version-buttons {
        margin-top: 1;
    }
    """

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="version-box"):
                yield Static("[bold]Step 2 of 3 -- Deephaven Version[/bold]")
                yield Static("")
                yield Static("Select a version to install:", id="version-prompt")
                yield ListView(id="version-list")
                yield Static("", id="version-status")
                with Horizontal(id="version-buttons"):
                    yield Button("Install", variant="primary", id="btn-install", disabled=True)

    def on_mount(self) -> None:
        self.run_worker(self._fetch_versions, thread=True)

    async def _fetch_versions(self) -> None:
        try:
            from deephaven_cli.manager.pypi import fetch_available_versions

            versions = fetch_available_versions()[:15]
            self.app.call_from_thread(self._populate_versions, versions)
        except Exception as e:
            self.app.call_from_thread(self._version_fetch_error, str(e))

    def _populate_versions(self, versions: list[str]) -> None:
        list_view = self.query_one("#version-list", ListView)
        for i, v in enumerate(versions):
            label = f"  {v}" + ("  [dim]latest[/dim]" if i == 0 else "")
            list_view.append(ListItem(Label(label), id=f"ver-{v}"))
        if versions:
            self._selected_version = versions[0]
            self.query_one("#btn-install", Button).disabled = False

    def _version_fetch_error(self, error: str) -> None:
        status = self.query_one("#version-status", Static)
        status.update(f"[red]Failed to fetch versions: {error}[/red]")

    def on_list_view_selected(self, event: ListView.Selected) -> None:
        item_id = event.item.id or ""
        if item_id.startswith("ver-"):
            self._selected_version = item_id[4:]

    def on_button_pressed(self, event: Button.Pressed) -> None:
        if event.button.id == "btn-install":
            version = getattr(self, "_selected_version", None)
            if version:
                self.app.push_screen(InstallProgressScreen(version))


class InstallProgressScreen(Screen):
    """Screen 4: Install progress.

    Args:
        version: The version to install.
        pop_count: How many screens to pop on success. 0 = wizard flow (push DoneScreen).
    """

    CSS = """
    InstallProgressScreen {
        align: center middle;
    }

    #install-box {
        width: 56;
        height: auto;
        border: double $primary;
        padding: 2 4;
    }

    #install-box Static {
        width: 100%;
    }

    #install-progress {
        margin: 1 0;
    }
    """

    BINDINGS = [("escape", "go_back_if_done", "Back")]

    def __init__(self, version: str, pop_count: int = 0) -> None:
        super().__init__()
        self.version = version
        self.pop_count = pop_count
        self._done = False

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="install-box"):
                yield Static(f"[bold]Installing Deephaven {self.version}...[/bold]")
                yield Static("")
                yield ProgressBar(total=100, id="install-progress")
                yield Static("Starting installation...", id="install-status")

    def on_mount(self) -> None:
        self.run_worker(self._do_install, thread=True)

    async def _do_install(self) -> None:
        from deephaven_cli.manager.config import get_default_version, set_default_version
        from deephaven_cli.manager.versions import install_version

        step_idx = [0]

        def on_progress(msg: str) -> None:
            step_idx[0] += 1
            self.app.call_from_thread(self._update_progress, msg, min(step_idx[0] * 30, 90))

        try:
            success = install_version(self.version, on_progress=on_progress)
            if success:
                if get_default_version() is None:
                    set_default_version(self.version)
                self.app.call_from_thread(self._install_done, None)
            else:
                self.app.call_from_thread(self._install_done, "Installation failed")
        except Exception as e:
            self.app.call_from_thread(self._install_done, str(e))

    def _update_progress(self, message: str, progress: int) -> None:
        self.query_one("#install-status", Static).update(message)
        self.query_one("#install-progress", ProgressBar).update(progress=progress)

    def _install_done(self, error: str | None) -> None:
        self._done = True
        if error:
            self.query_one("#install-status", Static).update(
                f"[red]{error}[/red]\n[dim]press esc to go back[/dim]"
            )
            self.query_one("#install-progress", ProgressBar).update(progress=100)
        else:
            self.query_one("#install-progress", ProgressBar).update(progress=100)
            if self.pop_count > 0:
                for _ in range(self.pop_count):
                    self.app.pop_screen()
                if hasattr(self.app.screen, "_refresh"):
                    self.app.screen._refresh()
            else:
                self.app.push_screen(DoneScreen(self.version))

    def action_go_back_if_done(self) -> None:
        if self._done:
            self.app.pop_screen()


class DoneScreen(Screen):
    """Screen 5: Setup complete."""

    CSS = """
    DoneScreen {
        align: center middle;
    }

    #done-box {
        width: 56;
        height: auto;
        border: double $primary;
        padding: 2 4;
    }

    #done-box Static {
        text-align: center;
        width: 100%;
    }

    #done-buttons {
        margin-top: 2;
    }

    #done-buttons Button {
        margin-right: 2;
    }
    """

    def __init__(self, version: str) -> None:
        super().__init__()
        self.version = version

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="done-box"):
                yield Static("[bold green]Setup Complete![/bold green]")
                yield Static("")
                yield Static(f"Deephaven {self.version} installed and set as default.")
                yield Static("")
                yield Static("[bold]Quick start:[/bold]")
                yield Static("  dh repl             Interactive REPL")
                yield Static("  dh exec script.py   Run a script")
                yield Static("  dh serve app.py     Start a server")
                yield Static("")
                with Center():
                    with Horizontal(id="done-buttons"):
                        yield Button("Launch REPL", variant="primary", id="btn-launch-repl")
                        yield Button("Done", id="btn-done")

    def on_button_pressed(self, event: Button.Pressed) -> None:
        if event.button.id == "btn-launch-repl":
            self.app.exit(result="launch-repl")
        elif event.button.id == "btn-done":
            self.app.exit()


# ---------------------------------------------------------------------------
# Main Menu Screen
# ---------------------------------------------------------------------------


class MainMenuScreen(Screen):
    """Interactive main menu when versions are already installed."""

    CSS = """
    MainMenuScreen {
        align: center middle;
    }

    #menu-box {
        width: 56;
        height: auto;
        border: double $primary;
        padding: 1 2;
    }

    #menu-header {
        width: 100%;
        text-align: center;
        margin-bottom: 1;
    }

    #menu-list {
        height: auto;
        margin: 0 1;
    }

    """

    BINDINGS = [
        ("r", "select_option('repl')", "REPL"),
        ("s", "select_option('serve')", "Serve"),
        ("e", "select_option('exec')", "Execute"),
        ("v", "select_option('versions')", "Versions"),
        ("l", "select_option('servers')", "Servers"),
        ("j", "select_option('java')", "Java"),
        ("c", "select_option('config')", "Config"),
        ("q", "select_option('quit')", "Quit"),
    ]

    _MENU_ITEMS = [
        ("repl", "Start REPL", "r"),
        ("exec", "Execute a script", "e"),
        ("serve", "Serve a script", "s"),
        ("versions", "Manage versions", "v"),
        ("servers", "Running servers", "l"),
        ("java", "Java", "j"),
        ("config", "Config", "c"),
        ("quit", "Quit", "q"),
    ]

    @staticmethod
    def _menu_row(label: str, key: str) -> "RichTable":
        from rich.table import Table as RichTable

        t = RichTable(show_header=False, box=None, padding=0, expand=True)
        t.add_column(ratio=1)
        t.add_column(justify="right")
        t.add_row(label, f"[dim]{key}[/dim]")
        return t

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="menu-box"):
                yield Static("", id="menu-header")
                yield OptionList(
                    *[Option(self._menu_row(label, key), id=opt_id) for opt_id, label, key in self._MENU_ITEMS],
                    id="menu-list",
                )

    def on_mount(self) -> None:
        self._refresh_header()
        self.query_one("#menu-list", OptionList).focus()

    def _refresh_header(self) -> None:
        from deephaven_cli.manager.config import get_default_version
        from deephaven_cli.manager.java import detect_java

        header = self.query_one("#menu-header", Static)
        version = get_default_version() or "none"
        java_info = detect_java()
        java_str = f"Java {java_info['version']}" if java_info else "No Java"
        header.update(f"[bold]Deephaven CLI[/bold]  |  v{version}  |  {java_str}")

    def on_option_list_option_selected(self, event: OptionList.OptionSelected) -> None:
        self._handle_selection(event.option.id)

    def action_select_option(self, option_id: str) -> None:
        self._handle_selection(option_id)

    def _handle_selection(self, option_id: str | None) -> None:
        if option_id == "repl":
            self.app.exit(result="launch-repl")
        elif option_id == "serve":
            self.app.exit(result="launch-serve")
        elif option_id == "exec":
            self.app.exit(result="launch-exec")
        elif option_id == "versions":
            self.app.push_screen(VersionsScreen())
        elif option_id == "servers":
            self.app.push_screen(ServersScreen())
        elif option_id == "java":
            self.app.push_screen(JavaStatusScreen())
        elif option_id == "config":
            self.app.push_screen(ConfigScreen())
        elif option_id == "quit":
            self.app.exit()


class RemoteVersionPickerScreen(Screen):
    """Pick a remote version to install (launched from VersionsScreen)."""

    CSS = """
    RemoteVersionPickerScreen {
        align: center middle;
    }

    #remote-picker-box {
        width: 56;
        height: 70%;
        border: double $primary;
        padding: 1 2;
    }

    #remote-picker-title {
        width: 100%;
        text-align: center;
        margin-bottom: 1;
    }

    #remote-picker-list {
        height: 1fr;
        margin: 0 1;
    }

    #remote-picker-status {
        margin-top: 1;
        width: 100%;
        text-align: center;
    }
    """

    BINDINGS = [
        ("escape", "go_back", "Back"),
        ("enter", "select", "Install"),
    ]

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="remote-picker-box"):
                yield Static("[bold]Install Version[/bold]", id="remote-picker-title")
                yield OptionList(id="remote-picker-list")
                yield Static("[dim]Fetching versions...[/dim]", id="remote-picker-status")
        yield Footer()

    def on_mount(self) -> None:
        self.run_worker(self._fetch_versions, thread=True)

    async def _fetch_versions(self) -> None:
        try:
            from deephaven_cli.manager.pypi import fetch_available_versions
            from deephaven_cli.manager.versions import list_installed_versions

            available = fetch_available_versions()[:30]
            installed_set = {v["version"] for v in list_installed_versions()}
            self.app.call_from_thread(self._populate, available, installed_set)
        except Exception as e:
            self.app.call_from_thread(self._fetch_error, str(e))

    def _populate(self, versions: list[str], installed_set: set[str]) -> None:
        picker_list = self.query_one("#remote-picker-list", OptionList)
        picker_list.clear_options()
        for v in versions:
            if v in installed_set:
                picker_list.add_option(
                    Option(f"  {v}  [dim](installed)[/dim]", id=f"v-{v}", disabled=True)
                )
            else:
                label = f"  {v}" + ("  [dim]latest[/dim]" if v == versions[0] else "")
                picker_list.add_option(Option(label, id=f"v-{v}"))
        picker_list.focus()
        self.query_one("#remote-picker-status", Static).update("")

    def _fetch_error(self, error: str) -> None:
        self.query_one("#remote-picker-status", Static).update(f"[red]Error: {error}[/red]")

    def on_option_list_option_selected(self, event: OptionList.OptionSelected) -> None:
        option_id = event.option.id or ""
        if option_id.startswith("v-"):
            version = option_id[2:]
            self.app.push_screen(InstallProgressScreen(version, pop_count=2))

    def action_go_back(self) -> None:
        self.app.pop_screen()


class VersionsScreen(Screen):
    """Manage installed versions."""

    CSS = """
    VersionsScreen {
        align: center middle;
    }

    #versions-box {
        width: 62;
        height: 70%;
        border: double $primary;
        padding: 1 2;
    }

    #versions-title {
        width: 100%;
        text-align: center;
        margin-bottom: 1;
    }

    #version-list {
        height: 1fr;
        margin: 0 1;
    }

    #versions-status {
        width: 100%;
        text-align: center;
    }
    """

    BINDINGS = [
        ("escape", "go_back", "Back"),
        ("d", "set_default", "Default"),
        ("u", "uninstall", "Uninstall"),
        ("delete", "uninstall", "Uninstall"),
        ("i", "install_new", "Install"),
        ("r", "toggle_remote", "Remote"),
    ]

    def __init__(self) -> None:
        super().__init__()
        self._versions: list[dict] = []
        self._remote_versions: list[str] = []
        self._show_remote: bool = False

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="versions-box"):
                yield Static("[bold]Installed Versions[/bold]", id="versions-title")
                yield OptionList(id="version-list")
                yield Static("", id="versions-status")
        yield Footer()

    def on_mount(self) -> None:
        self._refresh()

    def _refresh(self) -> None:
        from deephaven_cli.manager.versions import list_installed_versions

        self._versions = list_installed_versions()
        version_list = self.query_one("#version-list", OptionList)
        version_list.clear_options()

        if not self._versions and not self._show_remote:
            version_list.add_option(
                Option("[dim]No versions installed. Press i to install.[/dim]", id="empty", disabled=True)
            )
            return

        for info in self._versions:
            marker = " (default)" if info["is_default"] else ""
            label = f"  {info['version']}{marker}  [dim]installed {info['installed_date']}[/dim]"
            version_list.add_option(Option(label, id=f"installed-{info['version']}"))

        if self._show_remote and self._remote_versions:
            version_list.add_option(Option("", id="separator", disabled=True))
            version_list.add_option(
                Option("[bold dim]Available from PyPI[/bold dim]", id="remote-header", disabled=True)
            )
            for v in self._remote_versions[:20]:
                version_list.add_option(Option(f"  [dim]{v}[/dim]", id=f"remote-{v}"))

        if version_list.option_count > 0:
            version_list.highlighted = 0
        version_list.focus()

    def _get_selected_version(self) -> dict | None:
        """Return the installed version dict for the highlighted item, or None."""
        version_list = self.query_one("#version-list", OptionList)
        highlighted = version_list.highlighted
        if highlighted is None or highlighted < 0 or highlighted >= len(self._versions):
            return None
        return self._versions[highlighted]

    def on_option_list_option_selected(self, event: OptionList.OptionSelected) -> None:
        option_id = event.option.id or ""
        if option_id.startswith("remote-"):
            version = option_id[7:]
            self.app.push_screen(InstallProgressScreen(version, pop_count=1))

    def action_set_default(self) -> None:
        version_info = self._get_selected_version()
        if version_info is None:
            return
        from deephaven_cli.manager.config import set_default_version

        version = version_info["version"]
        set_default_version(version)
        self.query_one("#versions-status", Static).update(f"[green]{version} set as default[/green]")
        self._refresh()

    def action_uninstall(self) -> None:
        version_info = self._get_selected_version()
        if version_info is None:
            return
        version = version_info["version"]

        from deephaven_cli.manager.config import (
            get_default_version,
            get_latest_installed_version,
            load_config,
            save_config,
            set_default_version,
        )
        from deephaven_cli.manager.versions import uninstall_version

        status = self.query_one("#versions-status", Static)

        if uninstall_version(version):
            status.update(f"[green]{version} uninstalled[/green]")
            if get_default_version() == version:
                latest = get_latest_installed_version()
                if latest:
                    set_default_version(latest)
                else:
                    config = load_config()
                    config.pop("default_version", None)
                    save_config(config)
        else:
            status.update(f"[red]{version} not found[/red]")

        self._refresh()

    def action_install_new(self) -> None:
        self.app.push_screen(RemoteVersionPickerScreen())

    def action_toggle_remote(self) -> None:
        self._show_remote = not self._show_remote
        status = self.query_one("#versions-status", Static)

        if self._show_remote:
            if not self._remote_versions:
                status.update("[dim]Fetching versions from PyPI...[/dim]")
                self.run_worker(self._fetch_remote_versions, thread=True)
                return
            self._refresh()
        else:
            self._remote_versions = []
            self._refresh()
            status.update("")

    async def _fetch_remote_versions(self) -> None:
        try:
            from deephaven_cli.manager.pypi import fetch_available_versions

            available = fetch_available_versions()
            installed_set = {v["version"] for v in self._versions}
            remote_only = [v for v in available if v not in installed_set]
            self.app.call_from_thread(self._on_remote_fetched, remote_only, None)
        except Exception as e:
            self.app.call_from_thread(self._on_remote_fetched, [], str(e))

    def _on_remote_fetched(self, versions: list[str], error: str | None) -> None:
        status = self.query_one("#versions-status", Static)
        if error:
            status.update(f"[red]Failed to fetch: {error}[/red]")
            self._show_remote = False
            return
        self._remote_versions = versions
        status.update(f"[dim]{len(versions)} versions available from PyPI[/dim]")
        self._refresh()

    def action_go_back(self) -> None:
        self.app.pop_screen()


class ServersScreen(Screen):
    """List running servers with kill and open-browser actions."""

    CSS = """
    ServersScreen {
        align: center middle;
    }

    #servers-box {
        width: 62;
        height: 70%;
        border: double $primary;
        padding: 1 2;
    }

    #servers-title {
        width: 100%;
        text-align: center;
        margin-bottom: 1;
    }

    #server-list {
        height: 1fr;
        margin: 0 1;
    }

    #servers-status {
        width: 100%;
        text-align: center;
    }
    """

    BINDINGS = [
        ("escape", "go_back", "Back"),
        ("k", "kill_server", "Kill"),
        ("delete", "kill_server", "Kill"),
        ("o", "open_browser", "Open"),
    ]

    def __init__(self) -> None:
        super().__init__()
        self._servers: list = []

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="servers-box"):
                yield Static("[bold]Running Servers[/bold]", id="servers-title")
                yield OptionList(id="server-list")
                yield Static("", id="servers-status")
        yield Footer()

    def on_mount(self) -> None:
        self._refresh()

    def _refresh(self) -> None:
        from deephaven_cli.discovery import discover_servers

        self._servers = discover_servers()
        server_list = self.query_one("#server-list", OptionList)
        server_list.clear_options()

        if not self._servers:
            server_list.add_option(Option("[dim]No running servers.[/dim]", id="empty", disabled=True))
            return

        for s in self._servers:
            script = f"  {s.script}" if s.script else ""
            label = f":{s.port:<6} pid {s.pid:<7} {s.source}{script}"
            server_list.add_option(Option(label, id=str(s.port)))

        if server_list.option_count > 0:
            server_list.highlighted = 0
        server_list.focus()

    def _get_selected_server(self):
        server_list = self.query_one("#server-list", OptionList)
        highlighted = server_list.highlighted
        if highlighted is None or highlighted < 0 or highlighted >= len(self._servers):
            return None
        return self._servers[highlighted]

    def on_option_list_option_selected(self, event: OptionList.OptionSelected) -> None:
        self.action_open_browser()

    def action_open_browser(self) -> None:
        server = self._get_selected_server()
        if server is None:
            return
        from deephaven_cli.cli import _open_browser

        url = f"http://localhost:{server.port}"
        _open_browser(url)
        self.query_one("#servers-status", Static).update(
            f"[green]Opened {url}[/green]"
        )

    def action_kill_server(self) -> None:
        server = self._get_selected_server()
        if server is None:
            return
        from deephaven_cli.discovery import kill_server

        success, message = kill_server(server.port)
        status = self.query_one("#servers-status", Static)
        if success:
            status.update(f"[green]{message}[/green]")
        else:
            status.update(f"[red]{message}[/red]")
        self._refresh()

    def action_go_back(self) -> None:
        self.app.pop_screen()


class JavaStatusScreen(Screen):
    """Show Java status."""

    CSS = """
    JavaStatusScreen {
        align: center middle;
    }

    #java-status-box {
        width: 56;
        height: auto;
        border: double $primary;
        padding: 2 4;
    }
    """

    BINDINGS = [("escape", "go_back", "Back")]

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="java-status-box"):
                yield Static("[bold]Java Status[/bold]")
                yield Static("", id="java-info")
                yield Static("")
                yield Static("[dim]esc: back[/dim]")

    def on_mount(self) -> None:
        from deephaven_cli.manager.java import detect_java

        info_widget = self.query_one("#java-info", Static)
        info = detect_java()
        if info:
            info_widget.update(
                f"[green]Java {info['version']}[/green]\n"
                f"  Path: {info['path']}\n"
                f"  Home: {info['home']}\n"
                f"  Source: {info['source']}"
            )
        else:
            info_widget.update(
                "[red]No compatible Java found (requires >= 17).[/red]\n\n"
                "Install with: dh java install"
            )

    def action_go_back(self) -> None:
        self.app.pop_screen()


class ConfigScreen(Screen):
    """Show and edit configuration."""

    CSS = """
    ConfigScreen {
        align: center middle;
    }

    #config-box {
        width: 56;
        height: auto;
        border: double $primary;
        padding: 2 4;
    }
    """

    BINDINGS = [("escape", "go_back", "Back")]

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="config-box"):
                yield Static("[bold]Configuration[/bold]")
                yield Static("", id="config-content")
                yield Static("")
                yield Static("[dim]esc: back[/dim]")

    def on_mount(self) -> None:
        from deephaven_cli.manager.config import load_config

        content = self.query_one("#config-content", Static)
        config = load_config()
        if config:
            lines = []
            for key, value in sorted(config.items()):
                lines.append(f"  {key} = {value}")
            content.update("\n".join(lines))
        else:
            content.update("[dim]No configuration set.[/dim]\n\nRun: dh config --set KEY VALUE")

    def action_go_back(self) -> None:
        self.app.pop_screen()


# ---------------------------------------------------------------------------
# Main Management App
# ---------------------------------------------------------------------------


class ManagementApp(App):
    """Textual app for Deephaven CLI management (dh with no args)."""

    TITLE = "Deephaven CLI"

    CSS = """
    Screen {
        background: $surface;
    }
    """

    BINDINGS = [
        ("ctrl+c", "quit", "Quit"),
        ("ctrl+q", "quit", "Quit"),
    ]

    def on_mount(self) -> None:
        from deephaven_cli.manager.config import get_installed_versions

        installed = get_installed_versions()
        if installed:
            self.push_screen(MainMenuScreen())
        else:
            self.push_screen(WelcomeScreen())


def run_management_tui() -> str | None:
    """Run the management TUI and return a result action string, or None.

    Possible return values:
    - "launch-repl": User wants to launch the REPL
    - "launch-serve": User wants to serve a script
    - "launch-exec": User wants to execute a script
    - None: User quit normally
    """
    app = ManagementApp()
    return app.run()
