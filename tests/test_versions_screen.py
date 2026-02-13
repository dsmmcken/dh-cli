"""Tests for the interactive VersionsScreen TUI."""
from __future__ import annotations

from unittest.mock import patch

import pytest
from textual.app import App, ComposeResult
from textual.screen import Screen
from textual.widgets import Footer, OptionList, Static

from deephaven_cli.tui.app import (
    InstallProgressScreen,
    RemoteVersionPickerScreen,
    VersionsScreen,
)

pytestmark = pytest.mark.asyncio

# ---------------------------------------------------------------------------
# Fake data
# ---------------------------------------------------------------------------

FAKE_INSTALLED = [
    {"version": "0.37.0", "installed_date": "2025-02-10T14:00:00", "is_default": True},
    {"version": "0.36.0", "installed_date": "2025-02-09T10:00:00", "is_default": False},
]

FAKE_AVAILABLE = ["0.37.0", "0.36.0", "0.35.0", "0.34.0", "0.33.0"]

# Patch targets (imports happen inside methods via lazy imports)
_PATCH_LIST = "deephaven_cli.manager.versions.list_installed_versions"
_PATCH_UNINSTALL = "deephaven_cli.manager.versions.uninstall_version"
_PATCH_SET_DEFAULT = "deephaven_cli.manager.config.set_default_version"
_PATCH_GET_DEFAULT = "deephaven_cli.manager.config.get_default_version"
_PATCH_GET_LATEST = "deephaven_cli.manager.config.get_latest_installed_version"
_PATCH_LOAD_CONFIG = "deephaven_cli.manager.config.load_config"
_PATCH_SAVE_CONFIG = "deephaven_cli.manager.config.save_config"
_PATCH_AVAILABLE = "deephaven_cli.manager.pypi.fetch_available_versions"
_PATCH_INSTALL = "deephaven_cli.manager.versions.install_version"


# ---------------------------------------------------------------------------
# Wrapper apps
# ---------------------------------------------------------------------------


class VersionsTestApp(App):
    def on_mount(self) -> None:
        self.push_screen(VersionsScreen())


class RemotePickerTestApp(App):
    def on_mount(self) -> None:
        self.push_screen(RemoteVersionPickerScreen())


# ---------------------------------------------------------------------------
# Tests: VersionsScreen
# ---------------------------------------------------------------------------


class TestVersionsScreen:

    async def test_shows_installed_versions(self):
        """Versions are listed in the OptionList."""
        with patch(_PATCH_LIST, return_value=list(FAKE_INSTALLED)):
            app = VersionsTestApp()
            async with app.run_test() as pilot:
                screen = app.screen
                assert isinstance(screen, VersionsScreen)

                option_list = screen.query_one("#version-list", OptionList)
                assert option_list.option_count == 2
                assert "0.37.0" in str(option_list.get_option_at_index(0).prompt)
                assert "default" in str(option_list.get_option_at_index(0).prompt)
                assert "0.36.0" in str(option_list.get_option_at_index(1).prompt)

    async def test_shows_empty_message(self):
        """When no versions installed, shows helpful message."""
        with patch(_PATCH_LIST, return_value=[]):
            app = VersionsTestApp()
            async with app.run_test() as pilot:
                option_list = app.screen.query_one("#version-list", OptionList)
                assert option_list.option_count == 1
                assert "No versions installed" in str(option_list.get_option_at_index(0).prompt)

    async def test_footer_is_rendered(self):
        """Footer widget is present on the screen."""
        with patch(_PATCH_LIST, return_value=list(FAKE_INSTALLED)):
            app = VersionsTestApp()
            async with app.run_test() as pilot:
                footer = app.screen.query_one(Footer)
                assert footer is not None

    async def test_set_default(self):
        """Pressing 'd' sets the first (highlighted) version as default."""
        with patch(_PATCH_LIST, return_value=list(FAKE_INSTALLED)):
            app = VersionsTestApp()
            async with app.run_test() as pilot:
                screen = app.screen

                with patch(_PATCH_SET_DEFAULT) as mock_set, patch(
                    _PATCH_LIST,
                    return_value=[
                        {"version": "0.37.0", "installed_date": "2025-02-10T14:00:00", "is_default": True},
                        {"version": "0.36.0", "installed_date": "2025-02-09T10:00:00", "is_default": False},
                    ],
                ):
                    await pilot.press("d")
                    await pilot.pause()

                    # First item (0.37.0) is highlighted by default
                    mock_set.assert_called_once_with("0.37.0")
                    status = screen.query_one("#versions-status", Static)
                    assert "0.37.0 set as default" in str(status._Static__content)

    async def test_uninstall(self):
        """Pressing 'u' uninstalls the first (highlighted) version."""
        with patch(_PATCH_LIST, return_value=list(FAKE_INSTALLED)):
            app = VersionsTestApp()
            async with app.run_test() as pilot:
                screen = app.screen

                with patch(_PATCH_UNINSTALL, return_value=True) as mock_uninstall, patch(
                    _PATCH_GET_DEFAULT, return_value="0.37.0"
                ), patch(
                    _PATCH_GET_LATEST, return_value="0.36.0"
                ), patch(
                    _PATCH_SET_DEFAULT
                ) as mock_set, patch(
                    _PATCH_LIST, return_value=[FAKE_INSTALLED[1]]
                ):
                    await pilot.press("u")
                    await pilot.pause()

                    # First item (0.37.0) was highlighted
                    mock_uninstall.assert_called_once_with("0.37.0")
                    # Since it was the default, a new default should be set
                    mock_set.assert_called_once_with("0.36.0")
                    status = screen.query_one("#versions-status", Static)
                    assert "0.37.0 uninstalled" in str(status._Static__content)

    async def test_uninstall_clears_default_when_none_left(self):
        """Uninstalling the last version clears the default."""
        single = [{"version": "0.37.0", "installed_date": "2025-02-10", "is_default": True}]
        with patch(_PATCH_LIST, return_value=list(single)):
            app = VersionsTestApp()
            async with app.run_test() as pilot:
                with patch(_PATCH_UNINSTALL, return_value=True), patch(
                    _PATCH_GET_DEFAULT, return_value="0.37.0"
                ), patch(
                    _PATCH_GET_LATEST, return_value=None
                ), patch(
                    _PATCH_LOAD_CONFIG, return_value={"default_version": "0.37.0"}
                ), patch(
                    _PATCH_SAVE_CONFIG
                ) as mock_save, patch(
                    _PATCH_LIST, return_value=[]
                ):
                    await pilot.press("u")
                    await pilot.pause()

                    # Config should be saved without default_version
                    mock_save.assert_called_once()
                    saved = mock_save.call_args[0][0]
                    assert "default_version" not in saved

    async def test_install_new_pushes_picker(self):
        """Pressing 'i' pushes RemoteVersionPickerScreen."""
        with patch(_PATCH_LIST, return_value=list(FAKE_INSTALLED)):
            app = VersionsTestApp()
            async with app.run_test() as pilot:
                with patch(_PATCH_AVAILABLE, return_value=FAKE_AVAILABLE), patch(
                    _PATCH_LIST, return_value=list(FAKE_INSTALLED)
                ):
                    await pilot.press("i")
                    await pilot.pause(delay=0.5)

                    assert isinstance(app.screen, RemoteVersionPickerScreen)

    async def test_toggle_remote(self):
        """Pressing 'r' fetches and shows remote versions."""
        with patch(_PATCH_LIST, return_value=list(FAKE_INSTALLED)):
            app = VersionsTestApp()
            async with app.run_test() as pilot:
                screen = app.screen

                with patch(_PATCH_AVAILABLE, return_value=FAKE_AVAILABLE):
                    await pilot.press("r")
                    await pilot.pause(delay=1.0)

                    assert screen._show_remote is True
                    option_list = screen.query_one("#version-list", OptionList)
                    # 2 installed + separator + header + 3 remote = 7
                    assert option_list.option_count == 7

                    # Toggle off
                    await pilot.press("r")
                    await pilot.pause()

                    assert screen._show_remote is False
                    assert option_list.option_count == 2

    async def test_escape_goes_back(self):
        """Pressing escape pops the screen."""
        with patch(_PATCH_LIST, return_value=list(FAKE_INSTALLED)):
            app = VersionsTestApp()
            async with app.run_test() as pilot:
                assert isinstance(app.screen, VersionsScreen)
                await pilot.press("escape")
                await pilot.pause()
                assert not isinstance(app.screen, VersionsScreen)

    async def test_no_crash_on_action_with_empty_list(self):
        """Actions on empty list don't crash."""
        with patch(_PATCH_LIST, return_value=[]):
            app = VersionsTestApp()
            async with app.run_test() as pilot:
                await pilot.press("d")
                await pilot.pause()
                await pilot.press("u")
                await pilot.pause()
                # No crash = pass


# ---------------------------------------------------------------------------
# Tests: RemoteVersionPickerScreen
# ---------------------------------------------------------------------------


class TestRemoteVersionPickerScreen:

    async def test_fetches_and_shows_versions(self):
        """Screen fetches PyPI versions and populates the list."""
        with patch(_PATCH_AVAILABLE, return_value=FAKE_AVAILABLE), patch(
            _PATCH_LIST, return_value=list(FAKE_INSTALLED)
        ):
            app = RemotePickerTestApp()
            async with app.run_test() as pilot:
                await pilot.pause(delay=1.0)

                screen = app.screen
                assert isinstance(screen, RemoteVersionPickerScreen)
                picker_list = screen.query_one("#remote-picker-list", OptionList)
                assert picker_list.option_count == 5

    async def test_installed_versions_are_disabled(self):
        """Already-installed versions are shown as disabled."""
        with patch(_PATCH_AVAILABLE, return_value=FAKE_AVAILABLE), patch(
            _PATCH_LIST, return_value=list(FAKE_INSTALLED)
        ):
            app = RemotePickerTestApp()
            async with app.run_test() as pilot:
                await pilot.pause(delay=1.0)

                picker_list = app.screen.query_one("#remote-picker-list", OptionList)
                # First two (0.37.0, 0.36.0) are installed → disabled
                assert picker_list.get_option_at_index(0).disabled is True
                assert "(installed)" in str(picker_list.get_option_at_index(0).prompt)
                assert picker_list.get_option_at_index(1).disabled is True
                # Third (0.35.0) is not installed → enabled
                assert picker_list.get_option_at_index(2).disabled is False

    async def test_escape_goes_back(self):
        """Pressing escape pops the picker screen."""
        with patch(_PATCH_AVAILABLE, return_value=FAKE_AVAILABLE), patch(
            _PATCH_LIST, return_value=list(FAKE_INSTALLED)
        ):
            app = RemotePickerTestApp()
            async with app.run_test() as pilot:
                await pilot.pause(delay=1.0)
                assert isinstance(app.screen, RemoteVersionPickerScreen)

                await pilot.press("escape")
                await pilot.pause()
                assert not isinstance(app.screen, RemoteVersionPickerScreen)

    async def test_fetch_error_shows_message(self):
        """Network error shows error in status."""
        with patch(
            _PATCH_AVAILABLE, side_effect=ConnectionError("No network")
        ), patch(
            _PATCH_LIST, return_value=list(FAKE_INSTALLED)
        ):
            app = RemotePickerTestApp()
            async with app.run_test() as pilot:
                await pilot.pause(delay=1.0)

                status = app.screen.query_one("#remote-picker-status", Static)
                assert "Error" in str(status._Static__content)

    async def test_footer_is_rendered(self):
        """Footer widget is present on the picker screen."""
        with patch(_PATCH_AVAILABLE, return_value=FAKE_AVAILABLE), patch(
            _PATCH_LIST, return_value=list(FAKE_INSTALLED)
        ):
            app = RemotePickerTestApp()
            async with app.run_test() as pilot:
                await pilot.pause(delay=1.0)
                footer = app.screen.query_one(Footer)
                assert footer is not None


# ---------------------------------------------------------------------------
# Tests: InstallProgressScreen
# ---------------------------------------------------------------------------


class TestInstallProgressScreen:

    async def test_pop_count_zero_pushes_done_screen(self):
        """Default pop_count=0 pushes DoneScreen on success (wizard flow)."""

        class TestApp(App):
            def on_mount(self):
                self.push_screen(InstallProgressScreen("0.35.0", pop_count=0))

        with patch(_PATCH_INSTALL, return_value=True), patch(
            _PATCH_GET_DEFAULT, return_value="0.37.0"
        ):
            app = TestApp()
            async with app.run_test() as pilot:
                await pilot.pause(delay=1.5)
                from deephaven_cli.tui.app import DoneScreen

                assert isinstance(app.screen, DoneScreen)

    async def test_pop_count_pops_screens(self):
        """pop_count > 0 pops screens and refreshes on success."""

        class DummyScreen(Screen):
            def __init__(self):
                super().__init__()
                self.refreshed = False

            def _refresh(self):
                self.refreshed = True

            def compose(self) -> ComposeResult:
                yield Static("Dummy")

        class TestApp(App):
            def __init__(self):
                super().__init__()
                self.dummy = DummyScreen()

            def on_mount(self):
                self.push_screen(self.dummy)
                self.push_screen(InstallProgressScreen("0.35.0", pop_count=1))

        with patch(_PATCH_INSTALL, return_value=True), patch(
            _PATCH_GET_DEFAULT, return_value="0.37.0"
        ):
            app = TestApp()
            async with app.run_test() as pilot:
                await pilot.pause(delay=1.5)
                assert isinstance(app.screen, DummyScreen)
                assert app.dummy.refreshed is True

    async def test_error_shows_message(self):
        """On install failure, shows error message."""

        class TestApp(App):
            def on_mount(self):
                self.push_screen(InstallProgressScreen("0.99.0", pop_count=1))

        with patch(_PATCH_INSTALL, return_value=False), patch(
            _PATCH_GET_DEFAULT, return_value="0.37.0"
        ):
            app = TestApp()
            async with app.run_test() as pilot:
                await pilot.pause(delay=1.5)
                assert isinstance(app.screen, InstallProgressScreen)
                status = app.screen.query_one("#install-status", Static)
                rendered = str(status._Static__content)
                assert "failed" in rendered.lower() or "esc" in rendered.lower()

    async def test_escape_after_error(self):
        """Can escape after install failure."""

        class DummyScreen(Screen):
            def compose(self) -> ComposeResult:
                yield Static("Dummy")

        class TestApp(App):
            def on_mount(self):
                self.push_screen(DummyScreen())
                self.push_screen(InstallProgressScreen("0.99.0", pop_count=1))

        with patch(_PATCH_INSTALL, return_value=False), patch(
            _PATCH_GET_DEFAULT, return_value="0.37.0"
        ):
            app = TestApp()
            async with app.run_test() as pilot:
                await pilot.pause(delay=1.5)
                assert isinstance(app.screen, InstallProgressScreen)

                await pilot.press("escape")
                await pilot.pause()
                assert isinstance(app.screen, DummyScreen)
