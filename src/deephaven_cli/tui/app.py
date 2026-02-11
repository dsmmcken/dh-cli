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
            self.call_from_thread(self._java_install_done, None)
        except Exception as e:
            self.call_from_thread(self._java_install_done, str(e))

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
            self.call_from_thread(self._populate_versions, versions)
        except Exception as e:
            self.call_from_thread(self._version_fetch_error, str(e))

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
    """Screen 4: Install progress."""

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

    def __init__(self, version: str) -> None:
        super().__init__()
        self.version = version

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

        steps = [
            "Creating virtual environment...",
            "Installing packages...",
        ]
        step_idx = [0]

        def on_progress(msg: str) -> None:
            step_idx[0] += 1
            self.call_from_thread(self._update_progress, msg, min(step_idx[0] * 30, 90))

        try:
            success = install_version(self.version, on_progress=on_progress)
            if success:
                if get_default_version() is None:
                    set_default_version(self.version)
                self.call_from_thread(self._install_done, None)
            else:
                self.call_from_thread(self._install_done, "Installation failed")
        except Exception as e:
            self.call_from_thread(self._install_done, str(e))

    def _update_progress(self, message: str, progress: int) -> None:
        self.query_one("#install-status", Static).update(message)
        self.query_one("#install-progress", ProgressBar).update(progress=progress)

    def _install_done(self, error: str | None) -> None:
        if error:
            self.query_one("#install-status", Static).update(f"[red]{error}[/red]")
            self.query_one("#install-progress", ProgressBar).update(progress=100)
        else:
            self.query_one("#install-progress", ProgressBar).update(progress=100)
            self.app.push_screen(DoneScreen(self.version))


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

    #servers-info {
        margin-top: 1;
        width: 100%;
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
        ("repl", "Start REPL"),
        ("exec", "Execute a script"),
        ("serve", "Serve a script"),
        ("versions", "Manage versions"),
        ("servers", "Running servers"),
        ("java", "Java"),
        ("config", "Config"),
        ("quit", "Quit"),
    ]

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="menu-box"):
                yield Static("", id="menu-header")
                yield OptionList(
                    *[Option(label, id=opt_id) for opt_id, label in self._MENU_ITEMS],
                    id="menu-list",
                )
                yield Static("", id="servers-info")

    def on_mount(self) -> None:
        self._refresh_header()
        self._refresh_servers()
        self.query_one("#menu-list", OptionList).focus()

    def _refresh_header(self) -> None:
        from deephaven_cli.manager.config import get_default_version
        from deephaven_cli.manager.java import detect_java

        header = self.query_one("#menu-header", Static)
        version = get_default_version() or "none"
        java_info = detect_java()
        java_str = f"Java {java_info['version']}" if java_info else "No Java"
        header.update(f"[bold]Deephaven CLI[/bold]  |  v{version}  |  {java_str}")

    def _refresh_servers(self) -> None:
        from deephaven_cli.discovery import discover_servers

        servers = discover_servers()
        info = self.query_one("#servers-info", Static)
        if servers:
            lines = ["[bold]Running servers:[/bold]"]
            lines.append(f"  {'PORT':<8} {'PID':<8} {'TYPE':<12}")
            for s in servers:
                lines.append(f"  {s.port:<8} {s.pid:<8} {s.source:<12}")
            info.update("\n".join(lines))
        else:
            info.update("[dim]No running servers.[/dim]")

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
            self._refresh_servers()
        elif option_id == "java":
            self.app.push_screen(JavaStatusScreen())
        elif option_id == "config":
            self.app.push_screen(ConfigScreen())
        elif option_id == "quit":
            self.app.exit()


class VersionsScreen(Screen):
    """Manage installed versions."""

    CSS = """
    VersionsScreen {
        align: center middle;
    }

    #versions-box {
        width: 60;
        height: auto;
        max-height: 80%;
        border: double $primary;
        padding: 2 4;
    }

    #versions-box Static {
        width: 100%;
    }

    #versions-list {
        margin: 1 0;
    }
    """

    BINDINGS = [("escape", "go_back", "Back")]

    def compose(self) -> ComposeResult:
        with Center():
            with Vertical(id="versions-box"):
                yield Static("[bold]Installed Versions[/bold]")
                yield Static("", id="versions-content")
                yield Static("")
                with Center():
                    yield Button("Back", id="btn-back")

    def on_mount(self) -> None:
        self._refresh()

    def _refresh(self) -> None:
        from deephaven_cli.manager.versions import list_installed_versions

        installed = list_installed_versions()
        content = self.query_one("#versions-content", Static)

        if not installed:
            content.update("[dim]No versions installed.[/dim]\n\nRun: dh install")
            return

        lines = []
        for info in installed:
            marker = " [bold](default)[/bold]" if info["is_default"] else ""
            lines.append(f"  {info['version']}{marker}  [dim]installed {info['installed_date']}[/dim]")
        content.update("\n".join(lines))

    def on_button_pressed(self, event: Button.Pressed) -> None:
        if event.button.id == "btn-back":
            self.action_go_back()

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
                with Center():
                    yield Button("Back", id="btn-back")

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

    def on_button_pressed(self, event: Button.Pressed) -> None:
        if event.button.id == "btn-back":
            self.action_go_back()

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
                with Center():
                    yield Button("Back", id="btn-back")

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

    def on_button_pressed(self, event: Button.Pressed) -> None:
        if event.button.id == "btn-back":
            self.action_go_back()

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
