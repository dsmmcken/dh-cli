from deephaven import ui
import deephaven.plot.express as dx
from deephaven import agg

iris = dx.data.iris(ticking=False)

ui_iris = ui.table(
    iris,
    reverse=True,
    front_columns=["Timestamp", "Species"],
    hidden_columns=["PetalLength", "PetalWidth", "SpeciesID"],
    density="compact",
)

scatter_by_species = dx.scatter(iris, x="SepalLength", y="SepalWidth", by="Species")

sepal_text = ui.text("SepalLength vs. SepalWidth By Species Panel")

sepal_flex = ui.flex(ui_iris, scatter_by_species)

sepal_flex_column = ui.flex(sepal_text, sepal_flex, direction="column")

sepal_length_hist = dx.histogram(iris, x="SepalLength", by="Species")
sepal_width_hist = dx.histogram(iris, x="SepalWidth", by="Species")

sepal_tabs = ui.tabs(
    ui.tab(sepal_flex, title="Sepal Length vs. Sepal Width"),
    ui.tab(sepal_length_hist, title="Sepal Length Histogram"),
    ui.tab(sepal_width_hist, title="Sepal Width Histogram"),
)
sepal_flex_tabs = ui.flex(sepal_text, sepal_tabs, direction="column")

about_markdown = ui.markdown(r"""
### Iris Dashboard

Explore the Iris dataset with **deephaven.ui**

- The data powering this dashboard is simulated Iris data
- Charts are from Deephaven Plotly Express
- Other components are from **deephaven.ui**
  """)

sepal_panel = ui.panel(sepal_flex_tabs, title="Sepal Panel")
about_panel = ui.panel(about_markdown, title="About")

iris_avg = iris.agg_by([agg.avg(cols=["SepalLength", "SepalWidth", "PetalLength", "PetalWidth"])], by=["Species"])
iris_max = iris.agg_by([agg.max_(cols=["SepalLength", "SepalWidth", "PetalLength", "PetalWidth"])], by=["Species"])
iris_min = iris.agg_by([agg.min_(cols=["SepalLength", "SepalWidth", "PetalLength", "PetalWidth"])], by=["Species"])

ui_iris_avg = ui.panel(iris_avg, title="Average")
ui_iris_max = ui.panel(iris_max, title="Max")
ui_iris_min = ui.panel(iris_min, title="Min")

species_table = iris.view("Species").select_distinct()

def create_sepal_panel(set_species):
  ui_iris = ui.table(
      iris,
      reverse=True,
      front_columns=["Timestamp", "Species"],
      hidden_columns=["PetalLength", "PetalWidth", "SpeciesID"],
      density="compact",
      on_row_double_press=lambda event: set_species(event["Species"]["value"])
    )

  sepal_flex = ui.flex(ui_iris, scatter_by_species)

  sepal_tabs = ui.tabs(
      ui.tab(sepal_flex, title="Sepal Length vs. Sepal Width"),
      ui.tab(sepal_length_hist, title="Sepal Length Histogram"),
      ui.tab(sepal_width_hist, title="Sepal Width Histogram"),
  )

  sepal_flex_tabs = ui.flex(sepal_text, sepal_tabs, direction="column")

  return ui.panel(sepal_flex_tabs, title="Sepal Panel")

@ui.component
def summary_badges(species):
  # Filter the tables to the selected species
  species_min = iris_min.where("Species=species")
  species_max = iris_max.where("Species=species")
  species_avg = iris_avg.where("Species=species")

  # Pull the desired columns from the tables before using the hooks
  sepal_length_min = ui.use_cell_data(species_min.view(["SepalLength"]))
  sepal_width_min = ui.use_cell_data(species_min.view(["SepalWidth"]))
  sepal_length_max = ui.use_cell_data(species_max.view(["SepalLength"]))
  sepal_width_max = ui.use_cell_data(species_max.view(["SepalWidth"]))
  sepal_length_avg = ui.use_cell_data(species_avg.view(["SepalLength"]))
  sepal_width_avg = ui.use_cell_data(species_avg.view(["SepalWidth"]))

  # format the values to 3 decimal places
  # set flex_grow to 0 to prevent the badges from growing
  return ui.flex(
    ui.badge(f"SepalLength Min: {sepal_length_min:.3f}", variant="info"),
    ui.badge(f"SepalLength Max: {sepal_length_max:.3f}", variant="info"),
    ui.badge(f"SepalLength Avg: {sepal_length_avg:.3f}", variant="info"),
    ui.badge(f"SepalWidth Min: {sepal_width_min:.3f}", variant="info"),
    ui.badge(f"SepalWidth Max: {sepal_width_max:.3f}", variant="info"),
    ui.badge(f"SepalWidth Avg: {sepal_width_avg:.3f}", variant="info"),
    flex_grow=0
  )

def create_heatmap(species):
    heatmap = ui.illustrated_message(
        ui.icon("vsFilter"),
        ui.heading("Species required"),
        ui.content("Select a species to display filtered table and chart."),
        width="100%",
    )

    if species:
        filtered_table = iris.where("Species = species")
        heatmap = dx.density_heatmap(filtered_table, x="SepalLength", y="SepalWidth")

    return heatmap

@ui.component
def create_species_dashboard():
    species, set_species = ui.use_state()
    species_picker = ui.picker(
        species_table,
        on_change=set_species,
        selected_key=species,
        label="Current Species",
    )

    heatmap = ui.use_memo(lambda: create_heatmap(species), [species])

    badges = summary_badges(species) if species else None

    species_panel = ui.panel(
        ui.flex(species_picker, badges, heatmap, direction="column"),
        title="Investigate Species",
    )

    sepal_panel = create_sepal_panel(set_species)

    return ui.column(
        ui.row(about_panel, ui.stack(ui_iris_avg, ui_iris_max, ui_iris_min), height=1),
        ui.row(sepal_panel, species_panel, height=2),
    )

iris_dashboard_widget = ui.dashboard(create_species_dashboard())
