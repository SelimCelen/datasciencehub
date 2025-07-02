

# 🔬 Scientific Data Processing Server

This is a plugin-driven scientific data processing backend built in **Go** using:

- 🧠 [Otto](https://github.com/robertkrimen/otto): JavaScript VM for executing user-defined logic
- 🚀 [Gin](https://github.com/gin-gonic/gin): High-performance web framework
- 🗃️ [MongoDB](https://www.mongodb.com/): Persistent storage for data jobs and plugins
- 📜 YAML-based task definition
- 🔁 Built-in MapReduce support

---

## 📦 Features

- Upload raw data and process it using chainable JavaScript plugins
- Store and manage JavaScript plugins in MongoDB
- Define complex workflows using YAML task files
- Run plugins sequentially or in parallel
- MapReduce support using JavaScript mappers/reducers
- RESTful API with full Swagger (OpenAPI 3.0) spec

---

## 🚀 Getting Started

### 1. Clone the repo

```bash
git clone https://github.com/SelimCelen/scientific-data-server.git
cd scientific-data-server
````

### 2. Setup Configuration

Create a `config.yaml` (optional):

```yaml
port: "8080"
mongo_uri: "mongodb://localhost:27017"
database_name: "scientific_data_processing"
```

Or use environment variables:

```bash
export SERVER_PORT=8080
export MONGO_URI=mongodb://localhost:27017
export DB_NAME=scientific_data_processing
```

### 3. Run the server

```bash
go run main.go
```

---

## 📡 API Endpoints

### 🔄 Data Processing

| Method | Path                        | Description                         |
| ------ | --------------------------- | ----------------------------------- |
| POST   | `/api/v1/data/upload`       | Upload raw data                     |
| POST   | `/api/v1/data/process`      | Apply plugin chain to uploaded data |
| POST   | `/api/v1/data/process/yaml` | Upload and run a YAML-defined task  |
| GET    | `/api/v1/data/jobs`         | List all data jobs                  |
| GET    | `/api/v1/data/jobs/:id`     | Get job details & results           |

### 🧩 Plugin Management

| Method | Path                            | Description               |
| ------ | ------------------------------- | ------------------------- |
| POST   | `/api/v1/plugins`               | Upload new plugin         |
| GET    | `/api/v1/plugins`               | List all plugins          |
| GET    | `/api/v1/plugins/:name`         | Get plugin details        |
| DELETE | `/api/v1/plugins/:name`         | Delete plugin             |
| POST   | `/api/v1/plugins/:name/execute` | Execute plugin with input |

---

## 🧪 Plugin Example

```json
{
  "name": "normalize",
  "description": "Normalize array by factor",
  "javascript": "var result = input.map(x => x / params.factor); result;"
}
```

---

## 📄 YAML Task Example

```yaml
name: Temperature Analysis
description: Normalize and threshold sensor data
parallel: false
steps:
  - name: normalize
    plugin: normalize
    params:
      factor: 100
    input:
      job_id: "64a7ff210e12123ab456789c"
  - name: threshold
    plugin: threshold
    params:
      limit: 0.5
```

---

## 📘 Swagger API Docs

You can find the OpenAPI (Swagger) specification in [`swagger.yaml`](swagger.yaml).
Preview it at [https://editor.swagger.io](https://editor.swagger.io).

---

## ⚙️ Dependencies

* Go 1.18+
* MongoDB
* Modules:

  * `github.com/gin-gonic/gin`
  * `github.com/robertkrimen/otto`
  * `go.mongodb.org/mongo-driver`
  * `gopkg.in/yaml.v3`

---

## 📌 To Do

* [ ] Add authentication (JWT or API key)
* [ ] Dockerize
* [ ] Frontend UI for job control
* [ ] Plugin update support
* [ ] Unit tests

---

## 🧑‍💻 Author

Made with ❤️ by \[Selim Çelen]

---

## 📄 License

MIT License – see [`LICENSE`](LICENSE) file for details.

```

---

Would you like me to:

- Create a minimal Dockerfile and `.dockerignore`?
- Add a `Makefile` or `run.sh` for simplified setup?

Let me know!
```
