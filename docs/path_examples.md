# Tags Examples

- `jsonr:"address.street"`
    - resolves the `street` field of the `address` object field from the top level
- `jsonr:"addresses.*.street"`
    - if `addresses` were an array of objects, this would resolve the `street` field from each address object in the `addresses` array
    - assuming the `street` field is a string this would result in the assignment to a struct field representing a `[]string` or similar compatible type
- `jsonr:"addresses.0.street"`
    - this would resolve `street` field from the 1st (zero based index) address object from the `addresses` array
- `jsonr:"address.[street,city,state]"`
    - resolves only the `street`, `city`, and `state` fields from the `address` object field
    - assuming the `street`, `city`, and `state` field values are of type `string` then the result in the assignment to a struct field representing
        - a map[string]string or similar compatible map value type ex. `map[string]string{ "street": "123 Main St", "city": "Anytown", "state": "CA" }`
        - a []string ex. `[]string{ "123 Main St", "Anytown", "CA"}`
- `jsonr:"addresses.[0:5].coordinates.[latitude,longitude]`
    - resolves the first 5 items of the addresses array
    - assuming the `latitude` and `longitude` fields are of type float64 the the assignment to a struct field representing:
        - a `[]float64` or `[2]float64` ex. `[]float64{ 37.774929, -122.419416}`
        - a `map[string]float64` ex. `map[string]float64{ "latitude": 37.774929, "longitude": -122.419416 }`

## Brief Example

In this example there is a json object representing an **Employee** that includes various details, but in this scenario we are only interested in the Employee's name, and the coordinates.

### Example JSON

```json
{
    "name": "John Doe",
    "age": 30,
    "email": "john.doe@example.com",
    "address": {
        "street": "123 Main St",
        "city": "Anytown",
        "state": "CA",
        "zip": "12345",
        "country": "USA",
        "coordinates": {
            "latitude": 37.774929,
            "longitude": -122.419416
        }
    }
}
```

### Traditional Structure

Here would be the traditional method for parsing the json to a struct to get the name and coordinates only. (could be included using nested structs in one declaration)

```go
type Employee struct {
	Name string `json:"name"`
	Address Address `json:"address"`
}

type Address struct {
	Coordinates Coordinates `json:"coordinates"`
}

type Coordinates struct {
	Latitude float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
```

### New Structure

```go
type EmployeeCoordinates struct {
	Name string `jsonr:"name"`
	Latitude float64 `jsonr:"address.coordinates.latitude"`
	Longitude float64 `jsonr:"address.coordinates.longitude"`
}

var result = EmployeeCoordinates{
    Name: "John Doe",
    Latitude: 37.774929,
    Longitude: -122.419416,
}
```

OR

```go
type EmployeeCoordinates struct {
	Name string `jsonr:"name"`
    Coordinates map[string]float64 `jsonr:"address.coordinates.[latitude,longitude]"`
}

var result = EmployeeCoordinates{
    Name: "John Doe",
    Coordinates: map[string]float64{
        "latitude": 37.774929,
        "longitude": -122.419416,
    },
}
```

OR

```go
type EmployeeCoordinates struct {
	Name string `jsonr:"name"`
    Coordinates [2]float64 `jsonr:"address.coordinates.[latitude,longitude]"`
}

var result = EmployeeCoordinates{
    Name: "John Doe",
    Coordinates: [2]float64{ 37.774929, -122.419416 },
}
```