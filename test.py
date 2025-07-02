import requests
import time
import yaml

API_URL = "http://localhost:8080/api/v1"

def upload_data(data):
    resp = requests.post(f"{API_URL}/data/upload", json=data)
    resp.raise_for_status()
    job_id = resp.json().get("id")
    print(f"Uploaded data, job ID: {job_id}")
    return job_id

def upload_plugin(name, description, javascript):
    plugin = {
        "name": name,
        "description": description,
        "javascript": javascript
    }
    resp = requests.post(f"{API_URL}/plugins", json=plugin)
    if resp.status_code != 201:
        print("Plugin upload failed:", resp.status_code, resp.text)
    resp.raise_for_status()
    print(f"Uploaded plugin '{name}'")

def process_data(job_id, plugins):
    payload = {
        "job_id": job_id,
        "plugins": plugins
    }
    resp = requests.post(f"{API_URL}/data/process", json=payload)
    resp.raise_for_status()
    results = resp.json().get("results")
    print(f"Processed data results: {results}")
    return results

def upload_and_process_yaml_task(yaml_content):
    files = {
        "yaml_file": ("task.yaml", yaml_content, "text/yaml")
    }
    resp = requests.post(f"{API_URL}/data/process/yaml", files=files)
    resp.raise_for_status()
    result = resp.json()
    print(f"YAML task processed. Job ID: {result.get('job_id')}")
    print(f"Results: {result.get('results')}")
    return result

def main():
    # Step 1: Upload sample data
    sample_data = {"values": [100, 200, 300, 400, 500]}
    job_id = upload_data(sample_data)

    # Step 2: Upload pluginslt;"
    normalize_js = "var result = input.values.map(x => x / params.factor); result;"

    threshold_js = "var limit = params.limit || 0; var result = input.map(x => x > limit ? 1 : 0); result;"


    upload_plugin("normalize", "Normalize array by factor", normalize_js)
    upload_plugin("threshold", "Threshold values", threshold_js)

    # Give the server a moment to cache plugins
    time.sleep(1)

    # Step 3: Process data with normalize plugin
    process_data(job_id, [{"name": "normalize", "params": {"factor": 100}}])

    # Step 4: Prepare YAML workflow content
    yaml_task = {
        "name": "Normalize and Threshold",
        "description": "Normalize data then apply threshold",
        "parallel": False,
        "steps": [
            {
                "name": "normalize_step",
                "plugin": "normalize",
                "params": {"factor": 100},
                "input": {"job_id": job_id}
            },
            {
                "name": "threshold_step",
                "plugin": "threshold",
                "params": {"limit": 2}
            }
        ]
    }
    yaml_content = yaml.dump(yaml_task)

    # Step 5: Upload and process YAML task
    upload_and_process_yaml_task(yaml_content)

if __name__ == "__main__":
    main()
