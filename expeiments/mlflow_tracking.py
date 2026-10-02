"""Shared MLflow setup for training, evaluation and offline inference."""

import os

import mlflow


def start_run(stage: str):
    """Configure the tracking server and return an active-run context manager."""
    tracking_uri = os.getenv("MLFLOW_TRACKING_URI", "http://localhost:5000")
    experiment = os.getenv("MLFLOW_EXPERIMENT_NAME", "vehicle-reid")
    mlflow.set_tracking_uri(tracking_uri)
    mlflow.set_experiment(experiment)
    return mlflow.start_run(run_name=os.getenv("MLFLOW_RUN_NAME", stage), tags={
        "project": "brightest-teeth",
        "stage": stage,
    })
