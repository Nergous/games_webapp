# API Endpoints Documentation

## Auth Endpoints

### Register User

-   **Path**: `/api/register`
-   **Method**: `POST`
-   **Content-Type**: `multipart/form-data`
-   **Request Body**:

    ```json
    {
        "email (string, required)": "User email",
        "password (string, required)": "User password",
        "steam_url (string, required)": "User Steam profile URL",
        "image (file, required)": "User profile photo"
    }
    ```

-   **Response**:
    -   Status: `200 OK`
    -   Body: Registered user ID (int64)

### Login User

-   **Path**: `/api/login`
-   **Method**: `POST`
-   **Content-Type**: `application/json`
-   **Request Body**:

    ```json
    {
        "email": "string",
        "password": "string",
        "app_id": 1
    }
    ```

-   **Response**:
    -   Status: `200 OK`
    -   Body: JWT token (string)
    -   Sets cookie: `auth_token`

### Get User Info

-   **Path**: `/api/games/user/info`
-   **Method**: `GET`
-   **Headers**:
    -   `Authorization: Bearer <token>`
-   **Response**:
    -   Status: `200 OK`
    -   Body:
        ```json
        {
            "email": "string",
            "steam_url": "string",
            "photo": "string"
        }
        ```

## Game Endpoints

### Get All Games

-   **Path**: `/api/games/`
-   **Method**: `GET`
-   **Response**:
    -   Status: `200 OK`
    -   Body: Array of Game objects

### Get Paginated Games for User

-   **Path**: `/api/games/user`
-   **Method**: `GET`
-   **Query Parameters**:
    -   `page` (int, optional, default=1) - Page number
    -   `page_size` (int, optional, default=10, max=100) - Items per page
-   **Headers**:
    -   `Authorization: Bearer <token>`
-   **Response**:
    -   Status: `200 OK`
    -   Body:
        ```json
        {
            "total": 0,
            "pages": 0,
            "current": 0,
            "size": 0,
            "data": []
        }
        ```

### Search All Games

-   **Path**: `/api/games/search?title={}`
-   **Method**: `GET`
-   **Query Parameters**:
    -   `title` (string, required) - Search query
-   **Response**:
    -   Status: `200 OK`
    -   Body: Array of matching Game objects

### Search User Games

-   **Path**: `/api/games/user/search?title={}`
-   **Method**: `GET`
-   **Query Parameters**:
    -   `title` (string, required) - Search query
-   **Headers**:
    -   `Authorization: Bearer <token>`
-   **Response**:
    -   Status: `200 OK`
    -   Body: Array of matching Game objects

### Get Game by ID

-   **Path**: `/api/games/{id}`
-   **Method**: `GET`
-   **Response**:
    -   Status: `200 OK`
    -   Body: Single Game object

### Create Game

-   **Path**: `/api/games/`
-   **Method**: `POST`
-   **Content-Type**: `multipart/form-data`
-   **Headers**:
    -   `Authorization: Bearer <token>`
-   **Request Body**:
    -   `title` (string, required)
    -   `preambula` (string)
    -   `developer` (string)
    -   `publisher` (string)
    -   `year` (string)
    -   `genre` (string)
    -   `url` (string)
    -   `priority` (int, 0-10)
    -   `status` (string)
    -   `image` (file, required)
-   **Response**:
    -   Status: `200 OK`
    -   Body: Created Game object

### Create Multiple Games from Wikipedia

-   **Path**: `/api/games/multi`
-   **Method**: `POST`
-   **Content-Type**: `application/json`
-   **Headers**:
    -   `Authorization: Bearer <token>`
-   **Request Body**:
    ```json
    {
        "names": ["string"]
    }
    ```
-   **Response**:
    -   Status: `201 Created` or `207 Multi-Status`
    -   Body:
        ```json
        {
            "success": [],
            "errors": []
        }
        ```

### Update Game

-   **Path**: `/api/games/{id}`
-   **Method**: `PUT`
-   **Content-Type**: `multipart/form-data`
-   **Headers**:
    -   `Authorization: Bearer <token>`
-   **Request Body**:
    -   `id` (int64, required)
    -   `title` (string)
    -   `preambula` (string)
    -   `developer` (string)
    -   `publisher` (string)
    -   `year` (string)
    -   `genre` (string)
    -   `url` (string)
    -   `priority` (int, 0-10)
    -   `status` (string)
    -   `created_at` (string, RFC3339)
    -   `image` (file or string) - New file or existing filename
-   **Response**:
    -   Status: `200 OK`
    -   Body: Updated Game object

### Delete Game

-   **Path**: `/api/games/{id}`
-   **Method**: `DELETE`
-   **Headers**:
    -   `Authorization: Bearer <token>`
-   **Response**:
    -   Status: `200 OK`
    -   Body: None

## Models

### Game Object Structure

```json
{
    "id": 0,
    "title": "string",
    "preambula": "string",
    "image": "string",
    "developer": "string",
    "publisher": "string",
    "year": "string",
    "genre": "string",
    "url": "string",
    "created_at": "RFC3339 timestamp",
    "updated_at": "RFC3339 timestamp"
}
```

### Game Status Values

Possible values for `status` field:

-   `planned`
-   `playing`
-   `finished`
